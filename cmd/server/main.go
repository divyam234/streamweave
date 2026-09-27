package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamweave/internal/api"
	"streamweave/internal/api/gen"
	"streamweave/internal/config"
	"streamweave/internal/db"
	"streamweave/internal/engine"
	"streamweave/internal/httpserver"
	"streamweave/internal/metadata/cinemeta"
	stremioprotocol "streamweave/internal/protocol/stremio"
	"streamweave/internal/provider"
	"streamweave/internal/resolver"
	"streamweave/internal/safehttp"
	"streamweave/internal/secretbox"
	usenetnative "streamweave/internal/usenet/native"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	secrets, err := secretbox.NewFromHex(cfg.MasterKey)
	if err != nil {
		logger.Error("configure secret encryption", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := openDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	if pool != nil {
		defer pool.Close()
	}

	apiHandler, err := api.NewHandler(pool, secrets, cfg.AllowPrivateProviderEndpoints, cfg.DatabaseSchema)
	if err != nil {
		logger.Error("configure database schema", "error", err)
		os.Exit(1)
	}
	controlServer, err := gen.NewServer(apiHandler)
	if err != nil {
		logger.Error("create generated API server", "error", err)
		os.Exit(1)
	}

	outboundClient := safehttp.New(safehttp.Config{
		AllowPrivate: cfg.AllowPrivateProviderEndpoints,
		Timeout:      15 * time.Second,
	})
	var nativeUsenet *usenetnative.Service
	if cfg.EnableNativeUsenet && secrets != nil {
		nativeUsenet, err = usenetnative.NewService(secrets, outboundClient, "")
		if err != nil {
			logger.Error("configure native usenet", "error", err)
			os.Exit(1)
		}
	}
	var providerSource engine.ProviderSource = engine.StaticSource{}
	var resolverSource engine.ResolverSource = engine.NoResolvers{}
	var proxyStreams *resolver.Source
	if pool != nil {
		queries, err := db.NewQueries(pool, cfg.DatabaseSchema)
		if err != nil {
			logger.Error("configure database schema", "error", err)
			os.Exit(1)
		}
		providerSource = provider.NewSource(
			queries,
			secrets,
			outboundClient,
			cfg.AllowPrivateProviderEndpoints,
		)
		proxyStreams = resolver.NewSource(queries, secrets, outboundClient, resolver.BaseURLs{
			AllDebrid:   cfg.ResolverURLs.AllDebrid,
			RealDebrid:  cfg.ResolverURLs.RealDebrid,
			Premiumize:  cfg.ResolverURLs.Premiumize,
			EasyDebrid:  cfg.ResolverURLs.EasyDebrid,
			TorBox:      cfg.ResolverURLs.TorBox,
			DebridLink:  cfg.ResolverURLs.DebridLink,
			Offcloud:    cfg.ResolverURLs.Offcloud,
			Debrider:    cfg.ResolverURLs.Debrider,
			Torrin:      cfg.ResolverURLs.Torrin,
			PikPakUser:  cfg.ResolverURLs.PikPakUser,
			PikPakDrive: cfg.ResolverURLs.PikPakDrive,
		}, nativeUsenet)
		resolverSource = proxyStreams
	}

	aggregationEngine := engine.NewWithSources(providerSource, resolverSource, 8)
	aggregationEngine.SetLogger(logger)
	aggregationEngine.SetMetadataSource(cinemeta.New("", outboundClient))
	stremioHandler := stremioprotocol.NewHandlerWithSecureLinks(aggregationEngine, cfg.Production).WithSecrets(secrets)

	ready := func(ctx context.Context) error {
		if pool == nil {
			return errors.New("database is disabled")
		}
		return pool.Ping(ctx)
	}
	server := &http.Server{
		Addr: cfg.Address,
		Handler: httpserver.NewRouter(
			logger,
			controlServer,
			stremioHandler,
			nativeUsenet,
			proxyStreams,
			httpserver.SecurityConfig{
				AdminToken:    cfg.AdminToken,
				SecureCookies: cfg.Production,
			},
			ready,
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    32 << 10,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", cfg.Address)
		errCh <- server.ListenAndServe()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signalCh:
		logger.Info("shutdown requested", "signal", sig.String())
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}

func openDatabase(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, nil
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	config.MaxConns = 20
	config.MinIdleConns = 2
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
