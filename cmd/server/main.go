package main

import (
	"lambda_server/docs"
	"lambda_server/internal/auth"
	"lambda_server/internal/config"
	"lambda_server/internal/database"
	"lambda_server/internal/handlers"
	"lambda_server/internal/server"
	"lambda_server/internal/services"
	"lambda_server/internal/store"
	"lambda_server/internal/websocket"
	"log"

	"github.com/joho/godotenv"
)

// @title           Lambda Server API
// @version         1.0
// @description     This is a Lambda Server API with authentication and WebSocket support.

// @contact.name   Github link
// @contact.url    https://github.com/Remake1/lambda-server

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:3000
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

func main() {
	// Load.env file into environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning:.env file not found")
	}

	cfg := config.LoadConfig()

	// Initialize Swagger docs
	docs.SwaggerInfo.BasePath = "/api/v1"
	docs.SwaggerInfo.Host = "localhost:3000"

	database.Connect(cfg)

	// Initialize auth module with config
	auth.Init(cfg)

	// AutoMigrate the schema
	err = database.DB.AutoMigrate(&database.User{}, &database.Screenshot{})
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

	// Initialize Screenshot Store
	screenshotStore := store.NewScreenshotStore(database.DB)
	screenshotStore.StartCleanupRoutine()

	// Initialize AI Handler
	aiHandler := handlers.NewAIHandler(geminiService, screenshotStore)

	// Start the server
	hub := websocket.NewHub(screenshotStore)

	// 2. Run the Hub in its own goroutine
	go hub.Run()

	// 3. Pass the Hub to the router setup
	router := server.NewRouter(hub, aiHandler)

	port := cfg.Port
	if port == "" {
		port = "3000" // Default port if not specified
	}
	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server: ", err)
	}
}
