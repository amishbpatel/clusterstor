package config

import "os"

type Config struct {
	Environment        string
	HTTPAddr           string
	DatabaseURL        string
	PublicBaseURL      string
	APIBaseURL         string
	GoogleClientID     string
	GoogleClientSecret string
	TokenEncryptionKey string
}

func Load() Config {
	return Config{
		Environment: env("CLUSTERSTOR_ENV", "local"),
		HTTPAddr: env("CLUSTERSTOR_HTTP_ADDR", ":8080"),
		DatabaseURL: env("CLUSTERSTOR_DATABASE_URL", "postgres://clusterstor:clusterstor@localhost:5432/clusterstor?sslmode=disable"),
		PublicBaseURL: env("CLUSTERSTOR_PUBLIC_BASE_URL", "http://localhost:5173"),
		APIBaseURL: env("CLUSTERSTOR_API_BASE_URL", "http://localhost:8080"),
		GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		TokenEncryptionKey: os.Getenv("CLUSTERSTOR_TOKEN_ENCRYPTION_KEY"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
