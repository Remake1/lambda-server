package websocket

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	// *** CRITICAL: Default is 512 bytes. This MUST be
	// increased to handle image files. ***
	// We'll set this to 10 MB for now. Adjust as needed.
	maxMessageSize = 10 * 1024 * 1024
)

// Connection is a middleman between the websocket connection and the hub.
// It wraps the gorilla/websocket.Conn and adds a send channel.
type Connection struct {
	// The WebSocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	// The Hub.
	hub *Hub

	// The Client UUID this connection is associated with.
	ClientUUID string

	// The type of connection (client or hardware).
	isHardware bool
}

// NewClientConnection creates a new Connection for an authenticated Client.
func NewClientConnection(conn *websocket.Conn, hub *Hub, clientUUID string) *Connection {
	return &Connection{
		conn:       conn,
		send:       make(chan []byte, 256), // Buffered channel
		hub:        hub,
		ClientUUID: clientUUID,
		isHardware: false,
	}
}

// NewHardwareConnection creates a new Connection for a Hardware device.
func NewHardwareConnection(conn *websocket.Conn, hub *Hub, clientUUID string) *Connection {
	return &Connection{
		conn:       conn,
		send:       make(chan []byte, 256), // Buffered channel
		hub:        hub,
		ClientUUID: clientUUID,
		isHardware: true,
	}
}

// RegisterWithHub sends the connection to the correct hub registration channel.
func (c *Connection) RegisterWithHub() {
	if c.isHardware {
		c.hub.registerHardware <- c
	} else {
		c.hub.registerClient <- c
	}
}

// StartPumps spawns the read and write goroutines for the connection.
// This should be called once, right after the connection is created.
func (c *Connection) StartPumps() {
	go c.writePump()
	go c.readPump()
}

// readPump pumps messages from the websocket connection to the hub.
// The application runs readPump in a per-connection goroutine.
func (c *Connection) readPump() {
	// On exit, unregister the connection and close the websocket
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
		log.Printf("Connection: readPump for %s (Hardware: %t) EXITING", c.ClientUUID, c.isHardware)
	}()

	// Set the MaxMessageSize
	c.conn.SetReadLimit(maxMessageSize)

	// Set up the pong handler
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		// ReadMessage is the only read call.
		// It returns the message type (Text, Binary) and the payload.
		messageType, payload, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Connection: readPump error for %s: %v", c.ClientUUID, err)
			}
			break // Exit loop on any error
		}

		// Create the HubMessage
		hubMsg := &HubMessage{
			Type:       messageType,
			Payload:    payload,
			Connection: c,
			ClientUUID: c.ClientUUID,
		}

		// Send the message to the correct Hub processing channel
		// based on the connection type.
		if c.isHardware {
			c.hub.processHardwareMsg <- hubMsg
		} else {
			c.hub.processClientMsg <- hubMsg
		}
	}
}

// writePump pumps messages from the hub's 'send' channel to the websocket connection.
// The application runs writePump in a per-connection goroutine.
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)

	// On exit, stop the ticker and close the connection
	defer func() {
		ticker.Stop()
		c.conn.Close()
		log.Printf("Connection: writePump for %s (Hardware: %t) EXITING", c.ClientUUID, c.isHardware)
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// This is the *only* place we call WriteMessage.
			// The server *always* sends JSON
			// back to both parties (even the image response is a
			// JSON *about* the image).
			// We use TextMessage for this JSON.
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Printf("Connection: writePump error for %s: %v", c.ClientUUID, err)
				return // Exit on write error
			}

		case <-ticker.C:
			// Send a ping message to the peer
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Connection: writePump ping error for %s: %v", c.ClientUUID, err)
				return // Exit on ping error
			}
		}
	}
}
