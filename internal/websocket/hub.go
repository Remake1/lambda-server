package websocket

import (
	"log"

	"github.com/coder/websocket"
)

// Hub maintains the set of active connections and orchestrates the
// pairing of clients and hardware.
type Hub struct {
	// pendingClients holds authenticated clients waiting for hardware to connect.
	// Keyed by User UUID.
	pendingClients map[string]*ClientConnection

	// activeClientToHardware maps a paired client to its hardware.
	activeClientToHardware map[*ClientConnection]*HardwareConnection

	// activeHardwareToClient maps a paired hardware to its client.
	activeHardwareToClient map[*HardwareConnection]*ClientConnection

	// registerClient is a channel for clients to register.
	registerClient chan *ClientConnection

	// registerHardware is a channel for hardware to register and attempt pairing.
	registerHardware chan *HardwareConnection

	// unregister is a channel for any connection to unregister.
	unregister chan interface{}

	// relayToHardware is a channel for messages from a client to its hardware.
	relayToHardware chan *ClientMessage

	// relayToClient is a channel for messages from hardware to its client.
	relayToClient chan *HardwareMessage
}

// NewHub creates a new Hub instance, initializing all maps and channels.
func NewHub() *Hub {
	return &Hub{
		pendingClients:         make(map[string]*ClientConnection),
		activeClientToHardware: make(map[*ClientConnection]*HardwareConnection),
		activeHardwareToClient: make(map[*HardwareConnection]*ClientConnection),
		registerClient:         make(chan *ClientConnection),
		registerHardware:       make(chan *HardwareConnection),
		unregister:             make(chan interface{}),
		relayToHardware:        make(chan *ClientMessage),
		relayToClient:          make(chan *HardwareMessage),
	}
}

// Run starts the Hub's event loop as a goroutine.
// It must be launched once at application startup.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.registerClient:
			// A new client has connected. Add it to the pending map.
			h.pendingClients[client.UserID] = client
			log.Printf("Client registered, waiting for hardware. UserID: %s", client.UserID)

		case hardware := <-h.registerHardware:
			// A new hardware has connected. Attempt to pair it.
			if client, ok := h.pendingClients[hardware.ClientUUID]; ok {
				// Pair found. Move from pending to active.
				delete(h.pendingClients, hardware.ClientUUID)
				h.activeClientToHardware[client] = hardware
				h.activeHardwareToClient[hardware] = client
				log.Printf("Pairing successful. Client UserID: %s, Hardware for: %s", client.UserID, hardware.ClientUUID)

				// Optional: Send a confirmation message to both parties
				// client.send <-byte(`{"status": "paired"}`)
				// hardware.send <-byte(`{"status": "paired"}`)

			} else {
				// No pending client found for this UUID. Reject the hardware.
				log.Printf("Hardware connection rejected. No pending client for UUID: %s", hardware.ClientUUID)
				reason := "No pending client found for this UUID"
				hardware.conn.Close(websocket.StatusPolicyViolation, reason)
			}

		case msg := <-h.relayToHardware:
			// A client sent a message. Find its paired hardware and relay.
			if hardware, ok := h.activeClientToHardware[msg.Client]; ok {
				// Use a select for non-blocking send
				select {
				case hardware.send <- msg.Payload:
				default:
					// Hardware send buffer is full. Disconnect it.
					log.Printf("Hardware send channel full. Disconnecting. UserID: %s", msg.Client.UserID)
					h.cleanupConnection(hardware)
				}
			}

		case msg := <-h.relayToClient:
			// Hardware sent a message. Find its paired client and relay.
			if client, ok := h.activeHardwareToClient[msg.Hardware]; ok {
				select {
				case client.send <- msg.Payload:
				default:
					// Client send buffer is full. Disconnect it.
					log.Printf("Client send channel full. Disconnecting. UserID: %s", client.UserID)
					h.cleanupConnection(client)
				}
			}

		case conn := <-h.unregister:
			// A connection (client or hardware) has disconnected.
			// We must clean up its state and its pair's state.
			h.cleanupConnection(conn)
		}
	}
}

// cleanupConnection handles unregistration logic for any connection type.
func (h *Hub) cleanupConnection(conn interface{}) {
	switch c := conn.(type) {
	case *ClientConnection:
		// Check if the client was pending
		if _, ok := h.pendingClients[c.UserID]; ok {
			delete(h.pendingClients, c.UserID)
			log.Printf("Pending client unregistered. UserID: %s", c.UserID)
			c.cancel() // Cancel the context to stop its goroutines
			return
		}

		// Check if the client was active
		if hardware, ok := h.activeClientToHardware[c]; ok {
			log.Printf("Active client unregistered. UserID: %s", c.UserID)
			// Remove both ends of the pair from active maps
			delete(h.activeClientToHardware, c)
			delete(h.activeHardwareToClient, hardware)

			// Close the paired hardware connection
			hardware.conn.Close(websocket.StatusGoingAway, "Client disconnected")
			hardware.cancel() // Cancel hardware's context
			c.cancel()        // Cancel client's context
		}

	case *HardwareConnection:
		// Check if the hardware was active
		if client, ok := h.activeHardwareToClient[c]; ok {
			log.Printf("Active hardware unregistered. For Client UserID: %s", c.ClientUUID)
			// Remove both ends of the pair from active maps
			delete(h.activeHardwareToClient, c)
			delete(h.activeClientToHardware, client)

			// Close the paired client connection
			client.conn.Close(websocket.StatusGoingAway, "Hardware disconnected")
			client.cancel() // Cancel client's context
			c.cancel()      // Cancel hardware's context
		}
	}
}
