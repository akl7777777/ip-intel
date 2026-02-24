package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Server
	Port string
	Host string

	// Auth
	AuthKey string // Bearer token for authentication, empty = no auth

	// Cache
	CacheTTL time.Duration

	// Persistent cache (SQLite or MySQL)
	PersistentCache     bool
	PersistentCacheType string // "sqlite" or "mysql"
	PersistentCacheDSN  string // SQLite: file path; MySQL: DSN string
	PersistentCacheTTL  time.Duration

	// Local database
	MMDBPath string

	// Provider API keys
	IPInfoToken  string
	IPDataAPIKey string

	// Provider control
	EnabledProviders []string
}

func Load() *Config {
	cfg := &Config{
		Port:     envOrDefault("PORT", "9090"),
		Host:     envOrDefault("HOST", "0.0.0.0"),
		AuthKey:  os.Getenv("AUTH_KEY"),
		CacheTTL: envDurationOrDefault("CACHE_TTL_HOURS", 6) * time.Hour,
		MMDBPath: envOrDefault("MMDB_PATH", "data/GeoLite2-ASN.mmdb"),

		PersistentCache:     envBool("PERSISTENT_CACHE", false),
		PersistentCacheType: envOrDefault("PERSISTENT_CACHE_TYPE", "sqlite"),
		PersistentCacheDSN:  envOrDefault("PERSISTENT_CACHE_DSN", "data/ip-cache.db"),
		PersistentCacheTTL:  envDurationOrDefault("PERSISTENT_CACHE_TTL_DAYS", 7) * 24 * time.Hour,

		IPInfoToken:  os.Getenv("IPINFO_TOKEN"),
		IPDataAPIKey: os.Getenv("IPDATA_API_KEY"),
	}

	if providers := os.Getenv("ENABLED_PROVIDERS"); providers != "" {
		cfg.EnabledProviders = strings.Split(providers, ",")
	}

	return cfg
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDurationOrDefault(key string, def int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n)
		}
	}
	return time.Duration(def)
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "true" || v == "1" || v == "yes"
}
