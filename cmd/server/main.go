package main

import (
	"lambda_server/internal/auth"
	"lambda_server/internal/config"
	"lambda_server/internal/database"
	"lambda_server/internal/server"
	"lambda_server/internal/services"
	"lambda_server/internal/websocket"
	"log"

	"github.com/joho/godotenv"
)

func main() {
	// Load.env file into environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning:.env file not found")
	}

	cfg := config.LoadConfig()

	database.Connect(cfg)

	// Initialize auth module with config
	auth.Init(cfg)

	// AutoMigrate the schema
	err = database.DB.AutoMigrate(&database.User{})
	if err != nil {
		log.Fatal("Failed to migrate database: ", err)
	}
	log.Println("Database migrated successfully")

	// Initialize Gemini AI service
	geminiService, err := services.NewGeminiService(cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize Gemini service: %v. AI features will be disabled.", err)
		geminiService = nil // Continue without AI features
	} else {
		log.Println("Gemini AI service initialized successfully")
	}

	// Start the server
	hub := websocket.NewHub(geminiService)

	// 2. Run the Hub in its own goroutine
	go hub.Run()

	// 3. Pass the Hub to the router setup
	router := server.NewRouter(hub)

	port := cfg.Port
	if port == "" {
		port = "8080" // Default port if not specified
	}
	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server: ", err)
	}
}
