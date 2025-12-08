package server

import (
	"lambda_server/internal/auth"
	"lambda_server/internal/database"
	"lambda_server/internal/handlers"
	"lambda_server/internal/websocket"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func NewRouter(hub *websocket.Hub, aiHandler *handlers.AIHandler) *gin.Engine {
	router := gin.Default() // gin.Default() comes with Logger and Recovery middleware.

	// Add CORS middleware
	router.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Health check endpoints for Kubernetes/Docker/Load Balancers
	router.GET("/health", func(c *gin.Context) {
		// Check database connection
		sqlDB, err := database.DB.DB()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "failed to get database connection",
			})
			return
		}

		if err := sqlDB.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "database connection failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"database": "connected",
		})
	})

	// Readiness probe - indicates if the server is ready to accept traffic
	router.GET("/ready", func(c *gin.Context) {
		// Check if database is available
		sqlDB, err := database.DB.DB()
		if err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}

		if err := sqlDB.Ping(); err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}

		c.Status(http.StatusOK)
	})

	api := router.Group("/api/v1")
	{
		authRoutes := api.Group("/auth")
		{
			authRoutes.POST("/register", auth.Register)
			authRoutes.POST("/login", auth.Login)
			authRoutes.POST("/refresh", auth.RefreshToken)

			authRoutes.GET("/me", AuthMiddleware(), auth.GetUserInfo)
		}

		aiRoutes := api.Group("/ai")
		{
			aiRoutes.Use(AuthMiddleware())
			aiRoutes.POST("/analyze", aiHandler.Analyze)
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
