package websocket

import (
	"encoding/json"
	"log"

	"github.com/gorilla/websocket"
)

// HubMessage is a generic wrapper for messages processed by the Hub.
// This allows the originating connection and its message payload
// to be passed to the Hub's central processing loop.
type HubMessage struct {
	Type       int
	Payload    []byte
	Connection *Connection
	ClientUUID string
}

// ConnectionPair holds the paired client and hardware connections.
// This is the core data structure for managing the 1-to-1 relationship.
type ConnectionPair struct {
	Client   *Connection
	Hardware *Connection
}

// Hub manages the lifecycle of WebSocket connections and message routing.
// It uses channels to serialize all access, ensuring concurrency safety
// without complex mutex locking.[3, 4]
type Hub struct {
	// A map of client UUIDs to their paired connections.
	connections map[string]*ConnectionPair

	// Channel for registering a new client (browser) connection.
	registerClient chan *Connection

	// Channel for registering a new hardware connection.
	registerHardware chan *Connection

	// Channel for unregistering any connection (client or hardware).
	unregister chan *Connection

	// Channel for processing messages received from a client.
	processClientMsg chan *HubMessage

	// Channel for processing messages received from hardware.
	processHardwareMsg chan *HubMessage
}

// NewHub creates and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		connections:        make(map[string]*ConnectionPair),
		registerClient:     make(chan *Connection),
		registerHardware:   make(chan *Connection),
		unregister:         make(chan *Connection),
		processClientMsg:   make(chan *HubMessage),
		processHardwareMsg: make(chan *HubMessage),
	}
}

// Run starts the Hub's event-processing loop in its own goroutine.
// This is the single "actor" that owns and serializes access to the 'connections' map.
func (h *Hub) Run() {
	log.Println("WebSocket Hub: RUNNING")
	for {
		select {
		// Case 1: Register a new Client (Browser)
		case conn := <-h.registerClient:
			h.handleClientRegistration(conn)

		// Case 2: Register a new Hardware device
		case conn := <-h.registerHardware:
			h.handleHardwareRegistration(conn)

		// Case 3: Unregister any connection
		case conn := <-h.unregister:
			h.handleUnregistration(conn)

		// Case 4: Process a message from a Client
		case msg := <-h.processClientMsg:
			h.handleClientMessage(msg)

		// Case 5: Process a message from Hardware
		case msg := <-h.processHardwareMsg:
			h.handleHardwareMessage(msg)
		}
	}
}

// --- Private Hub Methods (executed by Run() goroutine) ---

func (h *Hub) handleClientRegistration(conn *Connection) {
	pair, ok := h.connections[conn.ClientUUID]
	if !ok {
		// This is the first party to connect. Create a new pair.
		pair = &ConnectionPair{}
		h.connections[conn.ClientUUID] = pair
	}

	// Check if a client is already connected for this UUID
	if pair.Client != nil {
		log.Printf("Hub: Warning: Client re-connection for UUID %s. Closing old connection.", conn.ClientUUID)
		// Close the old client's 'send' channel.
		// This triggers its writePump to exit, which in turn
		// triggers the readPump to exit and send an unregister event.
		close(pair.Client.send)
	}

	pair.Client = conn
	log.Printf("Hub: Client registered for UUID %s", conn.ClientUUID)

	// If hardware is already waiting, notify both parties
	if pair.Hardware != nil {
		h.notifyPairConnected(pair)
	}
}

func (h *Hub) handleHardwareRegistration(conn *Connection) {
	pair, ok := h.connections[conn.ClientUUID]
	if !ok {
		// This is the first party to connect. Create a new pair.
		pair = &ConnectionPair{}
		h.connections[conn.ClientUUID] = pair
	}

	// Check if hardware is already connected for this UUID
	if pair.Hardware != nil {
		log.Printf("Hub: Warning: Hardware re-connection for UUID %s. Closing old connection.", conn.ClientUUID)
		close(pair.Hardware.send)
	}

	pair.Hardware = conn
	log.Printf("Hub: Hardware registered for UUID %s", conn.ClientUUID)

	// If client is already waiting, notify both parties
	if pair.Client != nil {
		h.notifyPairConnected(pair)
	}
}

func (h *Hub) handleUnregistration(conn *Connection) {
	pair, ok := h.connections[conn.ClientUUID]
	if !ok {
		// This connection was never fully registered or already cleaned up.
		log.Printf("Hub: Warning: Unregistration for unknown UUID %s", conn.ClientUUID)
		return
	}

	if conn == pair.Client {
		// Client disconnected
		pair.Client = nil
		log.Printf("Hub: Client unregistered for UUID %s", conn.ClientUUID)
		// Notify hardware, if it's still connected.
		if pair.Hardware != nil {
			h.sendSystemMessage(pair.Hardware, "client_disconnected")
		}
	} else if conn == pair.Hardware {
		// Hardware disconnected
		pair.Hardware = nil
		log.Printf("Hub: Hardware unregistered for UUID %s", conn.ClientUUID)
		// Notify client, if it's still connected.
		if pair.Client != nil {
			h.sendSystemMessage(pair.Client, "hardware_disconnected")
		}
	}

	// If both parties are now disconnected, clean up the map entry
	if pair.Client == nil && pair.Hardware == nil {
		delete(h.connections, conn.ClientUUID)
		log.Printf("Hub: ConnectionPair for UUID %s removed.", conn.ClientUUID)
	}
}

func (h *Hub) handleClientMessage(msg *HubMessage) {
	pair, ok := h.connections[msg.ClientUUID]
	if !ok || pair.Hardware == nil {
		// No hardware to send to
		log.Printf("Hub: Dropping client message for %s. No hardware connected.", msg.ClientUUID)
		h.sendSystemMessage(msg.Connection, "hardware_not_connected")
		return
	}

	// Forward the client's message (JSON command) to the hardware [14]
	// The message is already a rawbyte (from the client's readPump).
	// We just place it in the hardware's send channel.
	log.Printf("Hub: Relaying C->H message for %s (%d bytes)", msg.ClientUUID, len(msg.Payload))

	// Use a select for a non-blocking send. If the hardware's
	// send buffer is full, we drop the message and log,
	// rather than blocking the entire Hub.
	select {
	case pair.Hardware.send <- msg.Payload:
	default:
		// Hardware's buffer is full. This indicates a problem.
		log.Printf("Hub: Error: Hardware send buffer full for %s. Closing connection.", msg.ClientUUID)
		close(pair.Hardware.send) // Triggers unregistration
	}
}

func (h *Hub) handleHardwareMessage(msg *HubMessage) {
	pair, ok := h.connections[msg.ClientUUID]
	if !ok || pair.Client == nil {
		// No client to send to
		log.Printf("Hub: Dropping hardware message for %s. No client connected.", msg.ClientUUID)
		return
	}

	// This is the core logic from the user query.
	// We must check the message type to distinguish
	// a data-file (image) from a status message (JSON).

	if msg.Type == websocket.BinaryMessage {
		// This is the image file [15, 16, 17]
		imageData := msg.Payload
		imageSize := len(imageData)

		log.Printf("Hub: Received BINARY image from hardware %s. Size: %d bytes", msg.ClientUUID, imageSize)

		// --- FUTURE AI INTEGRATION HOOK ---
		// For now, we just get the size.
		// In the future, this is where you'd call your AI service:
		//
		// aiResult, err := ai.ProcessImage(imageData)
		// if err!= nil {
		//   h.sendErrorMessage(pair.Client, "ai_processing_failed", err.Error())
		//   return
		// }
		//
		// Then, you would use 'aiResult' in the response.
		// --- END HOOK ---

		// Create the JSON response for the CLIENT
		response := map[string]interface{}{
			"type": "image_analysis_result",
			"payload": map[string]interface{}{
				"image_size": imageSize,
				// "ai_result": aiResult, // For the future
			},
		}

		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Printf("Hub: Error: Failed to marshal response: %v", err)
			return
		}

		// Send this JSON response to the Client's send channel
		select {
		case pair.Client.send <- jsonResponse:
		default:
			log.Printf("Hub: Error: Client send buffer full for %s. Closing connection.", msg.ClientUUID)
			close(pair.Client.send)
		}

	} else if msg.Type == websocket.TextMessage {
		// This is a JSON status or other text message from the hardware
		log.Printf("Hub: Received TEXT message from hardware %s: %s", msg.ClientUUID, string(msg.Payload))

		// For now, we wrap and forward this to the client as a status update.
		response := map[string]interface{}{
			"type":    "hardware_status_update",
			"payload": json.RawMessage(msg.Payload), // Assuming payload is valid JSON
		}
		jsonResponse, _ := json.Marshal(response)

		select {
		case pair.Client.send <- jsonResponse:
		default:
			close(pair.Client.send)
		}
	}
}

// --- Helper functions for notifications ---

func (h *Hub) notifyPairConnected(pair *ConnectionPair) {
	log.Printf("Hub: Pair connected for UUID %s", pair.Client.ClientUUID)
	h.sendSystemMessage(pair.Client, "hardware_connected")
	h.sendSystemMessage(pair.Hardware, "client_connected")
}

func (h *Hub) sendSystemMessage(conn *Connection, message string) {
	// A helper to send a structured system message
	msg := map[string]interface{}{
		"type": "system_status",
		"payload": map[string]string{
			"message": message,
		},
	}
	jsonMsg, _ := json.Marshal(msg)

	select {
	case conn.send <- jsonMsg:
	default:
		// Connection's buffer is full, disconnect it
		close(conn.send)
	}
}
