package websocket

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"lambda_server/internal/store"
	"lambda_server/internal/utils"
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
	// Client preferences for AI processing
	RequestType string // "leetcode" or "other"
	Language    string // "C++", "C", "Python", "JavaScript", "TypeScript"
	Model       string // "gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"
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

	// Screenshot store for temporary storage
	screenshotStore *store.ScreenshotStore
}

// NewHub creates and returns a new Hub instance.
func NewHub(screenshotStore *store.ScreenshotStore) *Hub {
	return &Hub{
		connections:        make(map[string]*ConnectionPair),
		registerClient:     make(chan *Connection),
		registerHardware:   make(chan *Connection),
		unregister:         make(chan *Connection),
		processClientMsg:   make(chan *HubMessage),
		processHardwareMsg: make(chan *HubMessage),
		screenshotStore:    screenshotStore,
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
	if !ok {
		// Create pair if it doesn't exist
		pair = &ConnectionPair{}
		h.connections[msg.ClientUUID] = pair
	}

	// Try to parse the message as JSON to extract type and language
	var clientMsg map[string]interface{}
	if err := json.Unmarshal(msg.Payload, &clientMsg); err == nil {
		// Extract type and language - check both top-level and nested in payload
		var requestType, language string

		// Check top-level first
		if rt, ok := clientMsg["type"].(string); ok {
			requestType = rt
		}
		if lang, ok := clientMsg["language"].(string); ok {
			language = lang
		}

		// If not found at top-level, check in payload
		if payload, ok := clientMsg["payload"].(map[string]interface{}); ok {
			if rt, ok := payload["type"].(string); ok && requestType == "" {
				requestType = rt
			}
			if lang, ok := payload["language"].(string); ok && language == "" {
				language = lang
			}
		}

		// Store request type if valid
		if requestType == "leetcode" || requestType == "other" {
			pair.RequestType = requestType
			log.Printf("Hub: Stored request type '%s' for UUID %s", requestType, msg.ClientUUID)
		}

		// Store language if valid
		if language != "" {
			validLanguages := map[string]bool{
				"C++":        true,
				"C":          true,
				"Python":     true,
				"JavaScript": true,
				"TypeScript": true,
			}
			if validLanguages[language] {
				pair.Language = language
				log.Printf("Hub: Stored language '%s' for UUID %s", language, msg.ClientUUID)
			}
		}

		// Extract and store model
		var model string
		if m, ok := clientMsg["model"].(string); ok {
			model = m
		} else if payload, ok := clientMsg["payload"].(map[string]interface{}); ok {
			if m, ok := payload["model"].(string); ok {
				model = m
			}
		}

		if model != "" {
			// Validate model
			validModels := map[string]bool{
				"gemini-2.5-pro":        true,
				"gemini-2.5-flash":      true,
				"gemini-2.5-flash-lite": true,
			}
			if validModels[model] {
				pair.Model = model
				log.Printf("Hub: Stored model '%s' for UUID %s", model, msg.ClientUUID)
			} else {
				log.Printf("Hub: Invalid model '%s' requested for UUID %s. Ignoring.", model, msg.ClientUUID)
				h.sendErrorMessage(msg.Connection, "invalid_model", fmt.Sprintf("Model '%s' is not supported.", model))
			}
		}
	}

	// If hardware is not connected, just store preferences and return
	if pair.Hardware == nil {
		log.Printf("Hub: Storing client preferences for %s. No hardware connected yet.", msg.ClientUUID)
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

	if msg.Type == websocket.BinaryMessage {
		// 1. Store the image locally
		imageData := msg.Payload
		imageID, err := h.screenshotStore.Save(msg.ClientUUID, imageData)
		if err != nil {
			log.Printf("Hub: Error saving screenshot for %s: %v", msg.ClientUUID, err)
			h.sendErrorMessage(pair.Client, "save_error", "Failed to save screenshot on server")
			return
		}

		// 2. Compress the image for the client
		compressedData, err := utils.CompressImage(imageData, 20) // 20% quality for preview
		if err != nil {
			log.Printf("Hub: Error compressing screenshot for %s: %v", msg.ClientUUID, err)
			// Decide if we should error out or send original (maybe too big).
			// Let's send error for now to be safe.
			h.sendErrorMessage(pair.Client, "compression_error", "Failed to compress screenshot")
			return
		}

		// Encode to Base64
		// We could send binary, but mixing JSON metadata with binary in WS is tricky without
		// a custom protocol. Sending JSON with base64 is easier for the client to handle
		// as a standard "event".
		base64Image := base64.StdEncoding.EncodeToString(compressedData)

		// 3. Send ID and Compressed Image to Client
		response := map[string]interface{}{
			"type": "screenshot_saved",
			"payload": map[string]interface{}{
				"id":    imageID,
				"image": base64Image,
			},
		}

		jsonResponse, err := json.Marshal(response)
		if err != nil {
			log.Printf("Hub: Error marshalling screenshot_saved response: %v", err)
			return
		}

		select {
		case pair.Client.send <- jsonResponse:
			log.Printf("Hub: Sent screenshot_saved (id: %s) to %s", imageID, msg.ClientUUID)
		default:
			log.Printf("Hub: Error: Client send buffer full for %s. Closing.", msg.ClientUUID)
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

func (h *Hub) sendErrorMessage(conn *Connection, errorType, errorMessage string) {
	// A helper to send error messages to the client
	msg := map[string]interface{}{
		"type": "error",
		"payload": map[string]string{
			"error_type":    errorType,
			"error_message": errorMessage,
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
