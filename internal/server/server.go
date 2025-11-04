package server

import (
	"lambda_server/internal/auth"

	"lambda_server/internal/websocket"

	"github.com/gin-gonic/gin"
)

func NewRouter(hub *websocket.Hub) *gin.Engine {
	router := gin.Default() // gin.Default() comes with Logger and Recovery middleware.

	api := router.Group("/api/v1")
	{
		authRoutes := api.Group("/auth")
		{
			authRoutes.POST("/register", auth.Register)
			authRoutes.POST("/login", auth.Login)
		}
	}

	// In server setup
	// Instantiate the WsHandler
	wsHandler := websocket.NewWsHandler(hub)

	// In server setup
	wsGroup := router.Group("/ws")
	{
		// Client endpoint protected by auth middleware
		authRequired := wsGroup.Group("/")

		authRequired.Use(AuthMiddleware())

		authRequired.GET("/client", wsHandler.ServeWsClient)

		// Temporary path without middleware for testing
		// wsGroup.GET("/client/:userID", func(c *gin.Context) {
		// 	// Mock auth middleware: set userID from path
		// 	c.Set("userID", c.Param("userID"))
		// 	wsHandler.ServeWsClient(c)
		// })

		// Hardware endpoint
		wsGroup.GET("/hardware/:uuid", wsHandler.ServeWsHardware)
	}

	return router
}
