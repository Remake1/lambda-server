// internal/websocket/handlers.go

package websocket

import (
	"context"
	"log"
	"net/http"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

// WsHandler holds the Hub and provides Gin handler methods.
type WsHandler struct {
	hub *Hub
}

// NewWsHandler creates a new WsHandler.
func NewWsHandler(h *Hub) *WsHandler {
	return &WsHandler{hub: h}
}

// ServeWsClient handles the client WebSocket connection request.
func (wh *WsHandler) ServeWsClient(c *gin.Context) {
	// 1. Get UserID from the Gin context (set by AuthMiddleware)
	userIDVal, ok := c.Get("userID")
	if !ok {
		log.Println("userID not found in context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		log.Println("userID in context is not a string")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// 2. Upgrade connection
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		// Set OriginPatterns for production environments
		// OriginPatterns:string{"example.com"},
	})
	if err != nil {
		log.Printf("Failed to upgrade client connection: %v", err)
		return
	}

	// 3. Create the master context for the connection's lifecycle
	ctx, cancel := context.WithCancel(context.Background())

	// 4. Create the ClientConnection struct
	client := &ClientConnection{
		conn:   conn,
		hub:    wh.hub,
		ctx:    ctx,
		cancel: cancel,
		send:   make(chan []byte, 256), // Buffered channel
		UserID: userID,
	}

	// 5. Register the client with the hub
	wh.hub.registerClient <- client

	// 6. Start the I/O goroutines
	go client.writePump()
	go client.readPump()
}

// ServeWsHardware handles the hardware WebSocket connection request.
func (wh *WsHandler) ServeWsHardware(c *gin.Context) {
	// 1. Get Client UUID from the URL parameter
	clientUUID := c.Param("uuid")
	if clientUUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing client uuid"})
		return
	}

	// 2. Upgrade connection
	conn, err := websocket.Accept(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade hardware connection: %v", err)
		return
	}

	// 3. Create the master context
	ctx, cancel := context.WithCancel(context.Background())

	// 4. Create the HardwareConnection struct
	hardware := &HardwareConnection{
		conn:       conn,
		hub:        wh.hub,
		ctx:        ctx,
		cancel:     cancel,
		send:       make(chan []byte, 256),
		ClientUUID: clientUUID,
	}

	// 5. Register the hardware with the hub (which will attempt pairing)
	wh.hub.registerHardware <- hardware

	// 6. Start the I/O goroutines
	go hardware.writePump()
	go hardware.readPump()
}
