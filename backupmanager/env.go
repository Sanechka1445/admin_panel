package backupmanager

import (
	"github.com/joho/godotenv"
	"os"
)

// LoadEnv loads environment variables from the given .env file path.
// Missing file is silently ignored (not an error).
func LoadEnv(path string) {
	_ = godotenv.Load(path)
}

// ConfigFromEnv constructs a Config from standard environment variables.
func ConfigFromEnv() Config {
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "3306"
	}
	return Config{
		Host:     getEnv("DB_HOST", "127.0.0.1"),
		Port:     port,
		User:     getEnv("DB_USER", "root"),
		Password: os.Getenv("DB_PASSWORD"),
		DBName:   os.Getenv("DB_NAME"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
