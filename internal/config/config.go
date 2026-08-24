// Package config reads deployment settings from the environment in one
// place. Pulling os.Getenv up here means no package deeper in the program
// reaches for the environment on its own, so behaviour is decided entirely
// by what the composition root passes down.
package config

import (
	"os"
	"time"
)

// Config is everything the server needs to know about where it is running.
type Config struct {
	DatabaseURL string
	RedisAddr   string
	ListenAddr  string
	SessionTTL  time.Duration

	// ShutdownGrace bounds how long the server waits for buffered logs to
	// reach the database before giving up and exiting.
	ShutdownGrace time.Duration
}

// Load reads configuration from the environment, applying defaults suited to
// running everything locally.
func Load() Config {
	return Config{
		DatabaseURL: env("DATABASE_URL", "postgres://localhost:5432/database?sslmode=disable"),
		RedisAddr:   env("REDIS_ADDR", "localhost:6379"),
		ListenAddr:  env("LISTEN_ADDR", ":50051"),
		SessionTTL:  24 * time.Hour,

		ShutdownGrace: 30 * time.Second,
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
