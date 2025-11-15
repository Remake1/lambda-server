package websocket

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// upgrader specifies the parameters for upgrading an HTTP connection
// to a WebSocket connection.
var upgrader = websocket.Upgrader{
	// Set reasonable buffer sizes.[1, 2, 5]
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,

	// CheckOrigin is a security-critical function.
	// In production, this must validate the origin against a
	// list of allowed domains.[25]
	CheckOrigin: func(r *http.Request) bool {
		// log.Printf("Upgrader: Checking origin: %s", r.Header.Get("Origin"))
		return true // Allow all for development.
	},

	// Enable compression for better performance.[16]
	EnableCompression: true,
}

// ServeClientWs is the Gin handler for the authenticated client's
// WebSocket connection.
// It retrieves the user's UUID from the AuthMiddleware.
// @Summary      Connect client WebSocket
// @Description  Establish a WebSocket connection for authenticated clients
// @Tags         websocket
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      101  "Switching Protocols"
// @Failure      401  {object}  map[string]string  "Unauthorized"
// @Failure      500  {object}  map[string]string  "Internal server error"
// @Router       /ws/client [get]
func (h *Hub) ServeClientWs(c *gin.Context) {
	// --- Extract data from Gin Context FIRST ---
	// 1. Get the Client UUID from the context.
	// This value *must* be set by your AuthMiddleware.
	// The key "userID" is assumed; this must match
	// the key used in the middleware's `c.Set()` call.
	uuidValue, exists := c.Get("userID")
	if !exists {
		log.Println("Handler: Error: userID not found in context. AuthMiddleware failed?")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	clientUUID, ok := uuidValue.(string)
	if !ok {
		log.Println("Handler: Error: userID in context is not a string.")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user identity in context"})
		return
	}
	// --- End Context Handling ---

	// 2. Upgrade the connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Handler: Failed to upgrade client connection: %v", err)
		return
	}
	log.Printf("Handler: Client connection upgrading for UUID %s", clientUUID)

	// 3. Create the Connection object
	wsConn := NewClientConnection(conn, h, clientUUID)

	// 4. Register with the Hub
	wsConn.RegisterWithHub()

	// 5. Start the read/write pumps
	wsConn.StartPumps()
}

// ServeHardwareWs is the Gin handler for the hardware's WebSocket
// connection. It retrieves the Client UUID from the URL path.
// @Summary      Connect hardware WebSocket
// @Description  Establish a WebSocket connection for hardware devices using client UUID
// @Tags         websocket
// @Accept       json
// @Produce      json
// @Param        uuid  path      string  true  "Client UUID"
// @Success      101   "Switching Protocols"
// @Failure      400   {object}  map[string]string  "Bad request"
// @Failure      500   {object}  map[string]string  "Internal server error"
// @Router       /ws/hardware/{uuid} [get]
func (h *Hub) ServeHardwareWs(c *gin.Context) {
	// --- Extract data from Gin Context FIRST ---
	// 1. Get the Client UUID from the path parameter.
	clientUUID := c.Param("uuid")
	if clientUUID == "" {
		log.Println("Handler: Error: Hardware connect with no UUID.")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing client UUID in path"})
		return
	}

	// Validate if it's a real UUID (optional but good practice)
	if _, err := uuid.Parse(clientUUID); err != nil {
		log.Printf("Handler: Error: Hardware connect with invalid UUID: %s", clientUUID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}
	// --- End Context Handling ---

	// 2. Upgrade the connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Handler: Failed to upgrade hardware connection: %v", err)
		return
	}
	log.Printf("Handler: Hardware connection upgrading for Client UUID %s", clientUUID)

	// 3. Create the Connection object
	wsConn := NewHardwareConnection(conn, h, clientUUID)

	// 4. Register with the Hub
	wsConn.RegisterWithHub()

	// 5. Start the read/write pumps
	wsConn.StartPumps()
}
