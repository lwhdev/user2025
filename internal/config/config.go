package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DBUser     string
	DBPassword string
	DBHost     string
	DBPort     int
	DBName     string
	JWTSecret  string
	ServerPort int
}

func Load() (*Config, error) {
	cfg := &Config{
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", "password"),
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBName:     getEnv("DB_NAME", "app_db"),
		JWTSecret:  getEnv("JWT_SECRET", "development-secret"),
	}

	port, err := strconv.Atoi(getEnv("DB_PORT", "3306"))
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}
	cfg.DBPort = port

	serverPort, err := strconv.Atoi(getEnv("SERVER_PORT", "8080"))
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_PORT: %w", err)
	}
	cfg.ServerPort = serverPort

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
