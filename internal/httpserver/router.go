package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"streamweave/internal/api/gen"
	stremioprotocol "streamweave/internal/protocol/stremio"
	"streamweave/internal/resolver"
	usenetnative "streamweave/internal/usenet/native"
	"streamweave/internal/webui"
)

func NewRouter(
	logger *slog.Logger,
	control *gen.Server,
	stremio *stremioprotocol.Handler,
	nativeUsenet *usenetnative.Service,
	proxyStreams *resolver.Source,
	security SecurityConfig,
	ready func(context.Context) error,
) http.Handler {
	sessions := newAdminSessionManager(security.AdminToken, security.SecureCookies)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders(security.SecureCookies))
	r.Use(accessLog(logger))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})

	if nativeUsenet != nil {
		r.With(publicCrossOrigin).Get("/api/v1/usenet/stream/{token}", func(w http.ResponseWriter, req *http.Request) {
			nativeUsenet.ServeToken(w, req, chi.URLParam(req, "token"))
		})
	}
	if proxyStreams != nil {
		stream := func(w http.ResponseWriter, req *http.Request) {
			proxyStreams.ServeStream(w, req, chi.URLParam(req, "token"))
		}
		r.With(publicCrossOrigin).Get("/api/v1/proxy/stream/{token}", stream)
		r.With(publicCrossOrigin).Head("/api/v1/proxy/stream/{token}", stream)
	}

	r.With(bodyLimit(8<<10)).Post("/auth/login", sessions.login)
	r.Get("/auth/session", sessions.session)
	r.Post("/auth/logout", sessions.logout)

	r.Group(func(admin chi.Router) {
		admin.Use(bodyLimit(maxControlBodyBytes))
		admin.Use(sessions.middleware)
		admin.Use(middleware.Timeout(20 * time.Second))
		admin.Handle("/api/v1/*", control)
	})

	r.Group(func(public chi.Router) {
		public.Use(publicCrossOrigin)
		public.Use(middleware.Timeout(30 * time.Second))
		public.Mount("/addon", stremio.Routes())
	})

	ui := webui.Handler()
	r.Handle("/", ui)
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, prefix := range []string{"/api/", "/auth/", "/addon/", "/healthz", "/readyz"} {
			if strings.HasPrefix(req.URL.Path, prefix) {
				http.NotFound(w, req)
				return
			}
		}
		ui.ServeHTTP(w, req)
	}))

	return r
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.InfoContext(r.Context(), "http request",
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method,
				"path", redactedPath(r.URL.Path),
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start),
			)
		})
	}
}

func redactedPath(path string) string {
	if strings.HasPrefix(path, "/addon/") {
		rest := strings.TrimPrefix(path, "/addon/")
		if index := strings.Index(rest, "/"); index >= 0 {
			if strings.HasPrefix(rest[index:], "/play/") {
				return "/addon/[token]/play/[token]"
			}
			return "/addon/[token]" + rest[index:]
		}
		return "/addon/[token]"
	}
	if strings.HasPrefix(path, "/api/v1/installations/") {
		rest := strings.TrimPrefix(path, "/api/v1/installations/")
		if index := strings.Index(rest, "/"); index >= 0 {
			return "/api/v1/installations/[token]" + rest[index:]
		}
		return "/api/v1/installations/[token]"
	}
	if strings.HasPrefix(path, "/api/v1/usenet/stream/") {
		return "/api/v1/usenet/stream/[token]"
	}
	if strings.HasPrefix(path, "/api/v1/proxy/stream/") {
		return "/api/v1/proxy/stream/[token]"
	}
	return path
}
