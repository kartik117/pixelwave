package config

import (
	"os"
)

type Config struct {
	Port            string
	RedisAddr       string
	PostgresDSN     string
	CanvasWidth     int
	CanvasHeight    int
	RateLimitWindow int // seconds
	CORSOrigin      string
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Load() Config {
	return Config{
		Port:            getEnv("PIXELWAVE_PORT", "8080"),
		RedisAddr:       getEnv("PIXELWAVE_REDIS_ADDR", "localhost:6379"),
		PostgresDSN:     getEnv("PIXELWAVE_POSTGRES_DSN", "postgres://pixelwave:pixelwave@localhost:5432/pixelwave?sslmode=disable"),
		CanvasWidth:     500,
		CanvasHeight:    500,
		RateLimitWindow: 1,
		CORSOrigin:      getEnv("PIXELWAVE_CORS_ORIGIN", "*"),
	}
}
