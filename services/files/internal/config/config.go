package config

import (
	"os"
	"time"
)

// Config holds all settings for the Files service. Values come only from
// environment variables (US-31), never from hardcoded addresses or secrets.
type Config struct {
	Port            string
	MongoURI        string
	MongoDB         string
	MinioEndpoint   string
	MinioAccessKey  string
	MinioSecretKey  string
	MinioBucket     string
	UsersServiceURL string
	RequestTimeout  time.Duration
}

func Load() Config {
	return Config{
		Port:            getEnv("FILES_PORT", "8082"),
		MongoURI:        getEnv("FILES_MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:         getEnv("FILES_MONGO_DB", "files"),
		MinioEndpoint:   getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKey:  os.Getenv("MINIO_ACCESS_KEY"),
		MinioSecretKey:  os.Getenv("MINIO_SECRET_KEY"),
		MinioBucket:     getEnv("MINIO_BUCKET", "files"),
		UsersServiceURL: getEnv("USERS_SERVICE_URL", "http://localhost:8081"),
		RequestTimeout:  getDuration("FILES_REQUEST_TIMEOUT", 5*time.Second),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
