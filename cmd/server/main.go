package main

import (
	"context"
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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// Configure database connection pool
	sqlDB, err := database.DB.DB()
	if err != nil {
		log.Fatal("Failed to get database connection: ", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

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

	// Run the Hub in its own goroutine
	go hub.Run()

	// Pass the Hub to the router setup
	router := server.NewRouter(hub, aiHandler)

	port := cfg.Port
	if port == "" {
		port = "3000" // Default port if not specified
	}

	// Create HTTP server with timeouts for production reliability
	// WriteTimeout is set to 110s to handle long-running AI requests (Gemini can take up to 60s)
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 110 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Channel to listen for OS signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-quit
	log.Println("Received shutdown signal, initiating graceful shutdown...")

	// Create shutdown context with 30 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown WebSocket hub first
	if err := hub.Shutdown(ctx); err != nil {
		log.Printf("WebSocket hub shutdown error: %v", err)
	}

	// Shutdown HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Close database connection
	if err := sqlDB.Close(); err != nil {
		log.Printf("Database close error: %v", err)
	}

	log.Println("Server shutdown complete")
}
