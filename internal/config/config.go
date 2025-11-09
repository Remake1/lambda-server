package config

import "os"

// Config holds all configuration for the application
type Config struct {
	GeminiAPIKey string
	JWTSecret    string
	Port         string
	DBHost       string
	DBUser       string
	DBPassword   string
	DBName       string
	DBPort       string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		GeminiAPIKey: os.Getenv("GOOGLE_API_KEY"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		Port:         os.Getenv("PORT"),
		DBHost:       os.Getenv("DB_HOST"),
		DBUser:       os.Getenv("DB_USER"),
		DBPassword:   os.Getenv("DB_PASSWORD"),
		DBName:       os.Getenv("DB_NAME"),
		DBPort:       os.Getenv("DB_PORT"),
	}
}
