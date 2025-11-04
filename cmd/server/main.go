package main

import (
	"lambda_server/internal/database"
	"lambda_server/internal/server"
	"log"
	"os"

	"lambda_server/internal/websocket"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading.env file")
	}

	database.Connect()

	// AutoMigrate the schema
	err = database.DB.AutoMigrate(&database.User{})
	if err != nil {
		log.Fatal("Failed to migrate database: ", err)
	}
	log.Println("Database migrated successfully")

	// Start the server
	hub := websocket.NewHub()

	// 2. Run the Hub in its own goroutine
	go hub.Run()

	// 3. Pass the Hub to the router setup
	router := server.NewRouter(hub)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port if not specified
	}
	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server: ", err)
	}
}
