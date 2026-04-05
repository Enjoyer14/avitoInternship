package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
}

func Load() Config {
	return Config{
		Port:        getEnv("APP_PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://avito:avito@localhost:5433/avito?sslmode=disable"),
		JWTSecret:   getEnv("JWT_SECRET", "ochen-secretniy-kluch"),
	}
}

func getEnv(key, other string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return other
}
