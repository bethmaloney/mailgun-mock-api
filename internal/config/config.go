package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	DBDriver    string
}

func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "8025"),
		DatabaseURL: getEnv("DATABASE_URL", "file:mailgun-mock.db"),
		DBDriver:    getEnv("DB_DRIVER", "sqlite"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
