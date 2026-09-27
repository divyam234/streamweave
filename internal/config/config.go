package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type ResolverURLs struct {
	AllDebrid   string
	RealDebrid  string
	Premiumize  string
	EasyDebrid  string
	TorBox      string
	DebridLink  string
	Offcloud    string
	Debrider    string
	Torrin      string
	PikPakUser  string
	PikPakDrive string
}

type Config struct {
	Address                       string
	DatabaseURL                   string
	MasterKey                     string
	AdminToken                    string
	Production                    bool
	EnableNativeUsenet            bool
	ResolverURLs                  ResolverURLs
	AllowPrivateProviderEndpoints bool
	ReadTimeout                   time.Duration
	WriteTimeout                  time.Duration
	IdleTimeout                   time.Duration
}

func Load() (Config, error) {
	allowPrivate, err := envBool("ALLOW_PRIVATE_PROVIDER_ENDPOINTS", false)
	if err != nil {
		return Config{}, err
	}
	production, err := envBool("PRODUCTION", false)
	if err != nil {
		return Config{}, err
	}
	enableNativeUsenet, err := envBool("ENABLE_NATIVE_USENET", false)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Address:            env("HTTP_ADDR", ":8080"),
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		MasterKey:          strings.TrimSpace(os.Getenv("MASTER_KEY")),
		AdminToken:         strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		Production:         production,
		EnableNativeUsenet: enableNativeUsenet,
		ResolverURLs: ResolverURLs{
			AllDebrid:   env("ALLDEBRID_BASE_URL", "https://api.alldebrid.com"),
			RealDebrid:  env("REALDEBRID_BASE_URL", "https://api.real-debrid.com/rest/1.0"),
			Premiumize:  env("PREMIUMIZE_BASE_URL", "https://www.premiumize.me/api"),
			EasyDebrid:  env("EASYDEBRID_BASE_URL", "https://easydebrid.com/api"),
			TorBox:      env("TORBOX_BASE_URL", "https://api.torbox.app"),
			DebridLink:  env("DEBRIDLINK_BASE_URL", "https://debrid-link.com/api"),
			Offcloud:    env("OFFCLOUD_BASE_URL", "https://offcloud.com"),
			Debrider:    env("DEBRIDER_BASE_URL", "https://debrider.app/api"),
			Torrin:      env("TORRIN_BASE_URL", "https://api.torrin.app"),
			PikPakUser:  env("PIKPAK_USER_BASE_URL", "https://user.mypikpak.com"),
			PikPakDrive: env("PIKPAK_DRIVE_BASE_URL", "https://api-drive.mypikpak.com"),
		},
		AllowPrivateProviderEndpoints: allowPrivate,
		ReadTimeout:                   10 * time.Second,
		WriteTimeout:                  30 * time.Second,
		IdleTimeout:                   60 * time.Second,
	}

	if value := os.Getenv("HTTP_READ_TIMEOUT_SECONDS"); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds <= 0 {
			return Config{}, fmt.Errorf("HTTP_READ_TIMEOUT_SECONDS must be a positive integer")
		}
		cfg.ReadTimeout = time.Duration(seconds) * time.Second
	}

	if cfg.Production {
		if cfg.DatabaseURL == "" {
			return Config{}, fmt.Errorf("DATABASE_URL is required when PRODUCTION=true")
		}
		if cfg.MasterKey == "" {
			return Config{}, fmt.Errorf("MASTER_KEY is required when PRODUCTION=true")
		}
		if len(cfg.AdminToken) < 32 {
			return Config{}, fmt.Errorf("ADMIN_TOKEN must be at least 32 characters when PRODUCTION=true")
		}
		if cfg.AllowPrivateProviderEndpoints {
			return Config{}, fmt.Errorf("ALLOW_PRIVATE_PROVIDER_ENDPOINTS must be false when PRODUCTION=true")
		}
		for name, raw := range map[string]string{
			"ALLDEBRID_BASE_URL":    cfg.ResolverURLs.AllDebrid,
			"REALDEBRID_BASE_URL":   cfg.ResolverURLs.RealDebrid,
			"PREMIUMIZE_BASE_URL":   cfg.ResolverURLs.Premiumize,
			"EASYDEBRID_BASE_URL":   cfg.ResolverURLs.EasyDebrid,
			"TORBOX_BASE_URL":       cfg.ResolverURLs.TorBox,
			"DEBRIDLINK_BASE_URL":   cfg.ResolverURLs.DebridLink,
			"OFFCLOUD_BASE_URL":     cfg.ResolverURLs.Offcloud,
			"DEBRIDER_BASE_URL":     cfg.ResolverURLs.Debrider,
			"TORRIN_BASE_URL":       cfg.ResolverURLs.Torrin,
			"PIKPAK_USER_BASE_URL":  cfg.ResolverURLs.PikPakUser,
			"PIKPAK_DRIVE_BASE_URL": cfg.ResolverURLs.PikPakDrive,
		} {
			parsed, parseErr := url.Parse(raw)
			if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" {
				return Config{}, fmt.Errorf("%s must be a valid https URL when PRODUCTION=true", name)
			}
		}
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return parsed, nil
}
