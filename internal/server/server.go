package server

import (
	"lambda_server/internal/auth"
	"lambda_server/internal/websocket"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func NewRouter(hub *websocket.Hub) *gin.Engine {
	router := gin.Default() // gin.Default() comes with Logger and Recovery middleware.

	// Add CORS middleware
	router.Use(cors.Default())

	api := router.Group("/api/v1")
	{
		authRoutes := api.Group("/auth")
		{
			authRoutes.POST("/register", auth.Register)
			authRoutes.POST("/login", auth.Login)
			authRoutes.POST("/refresh", auth.RefreshToken)

			authRoutes.GET("/me", AuthMiddleware(), auth.GetUserInfo)
		}
	}

	// Instantiate the WsHandler
	wsGroup := router.Group("/ws")
	{
		// Client endpoint protected by auth middleware
		authRequired := wsGroup.Group("/")
		authRequired.Use(AuthMiddleware())

		authRequired.GET("/client", func(c *gin.Context) {
			hub.ServeClientWs(c)
		})

		// Hardware endpoint
		wsGroup.GET("/hardware/:uuid", func(c *gin.Context) {
			hub.ServeHardwareWs(c)
		})
	}

	// Swagger documentation route
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

	return router
}
