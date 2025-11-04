// internal/websocket/client.go

package websocket

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
)

// ClientConnection is a wrapper around a websocket.Conn for an authenticated client.
type ClientConnection struct {
	conn *websocket.Conn
	hub  *Hub
	// The master context for this connection's lifetime.
	ctx context.Context
	// The function to call to cancel the context and kill the connection.
	cancel context.CancelFunc
	// Buffered channel of outbound (JSON) messages.
	send chan []byte
	// Authenticated User's UUID.
	UserID string
}

// HardwareConnection is a wrapper around a websocket.Conn for a hardware device.
type HardwareConnection struct {
	conn *websocket.Conn
	hub  *Hub
	// The master context for this connection's lifetime.
	ctx context.Context
	// The function to call to cancel the context and kill the connection.
	cancel context.CancelFunc
	// Buffered channel of outbound (JSON) messages.
	send chan []byte
	// The Client User UUID this hardware intends to pair with.
	ClientUUID string
}

// readPump pumps messages from the WebSocket connection to the hub.
// This implementation is for the CLIENT.
func (c *ClientConnection) readPump() {
	// Defer the unregistration and context cancellation.
	// This ensures cleanup happens when the read loop exits for any reason.
	defer func() {
		c.hub.unregister <- c
		c.cancel()
	}()

	for {
		var call ClientCallPayload
		// Use wsjson.Read for automatic JSON unmarshaling.
		// This blocks until a message is received or the context is cancelled.
		err := wsjson.Read(c.ctx, c.conn, &call)
		if err != nil {
			if websocket.CloseStatus(err) == -1 {
				log.Printf("Client read error: %v", err)
			}
			break // Exit loop on any error
		}

		// Marshal the payload back to raw JSON to send to the hub's relay.
		// This avoids the hub needing to know the struct type.
		payloadBytes, err := json.Marshal(call)
		if err != nil {
			log.Printf("Client message marshal error: %v", err)
			continue
		}

		// Send the message to the hub for relaying to hardware.
		msg := &ClientMessage{
			Client:  c,
			Payload: payloadBytes,
		}

		// Use a select to avoid blocking if the hub is busy
		select {
		case c.hub.relayToHardware <- msg:
		case <-c.ctx.Done():
			return
		}
	}
}

// readPump pumps messages from the WebSocket connection to the hub.
// This implementation is for the HARDWARE.
func (h *HardwareConnection) readPump() {
	defer func() {
		h.hub.unregister <- h
		h.cancel()
	}()

	for {
		// We must use the low-level Reader to handle binary messages.
		msgType, r, err := h.conn.Reader(h.ctx)
		if err != nil {
			if websocket.CloseStatus(err) == -1 {
				log.Printf("Hardware read error: %v", err)
			}
			break
		}

		switch msgType {
		case websocket.MessageText:
			// Hardware sent a text message. For now, we log and ignore it.
			// This could be used for heartbeats or status updates in the future.
			log.Println("Hardware sent unexpected text message.")
			// Must read to EOF to clear the message
			io.Copy(io.Discard, r)

		case websocket.MessageBinary:
			// This is the image file stream.
			log.Println("Hardware sent binary image file. Calculating size...")

			// We only need the size. Stream the entire message to io.Discard
			// to get the total byte count efficiently without buffering.
			size, err := io.Copy(io.Discard, r)
			if err != nil {
				log.Printf("Error reading binary stream from hardware: %v", err)
				continue
			}

			// We have the size. Create the response payload.
			props := ImagePropsPayload{
				Size: size,
			}
			payloadBytes, err := json.Marshal(props)
			if err != nil {
				log.Printf("ImageProps marshal error: %v", err)
				continue
			}

			// Send the image properties to the hub for relaying to the client.
			msg := &HardwareMessage{
				Hardware: h,
				Payload:  payloadBytes,
			}

			select {
			case h.hub.relayToClient <- msg:
			case <-h.ctx.Done():
				return
			}

		default:
			log.Printf("Hardware sent unknown message type: %v", msgType)
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
// This is a generic implementation for both Client and Hardware.
func (c *ClientConnection) writePump() {
	defer func() {
		// On exit, ensure the connection is closed.
		// The readPump handles unregistration.
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// The hub closed the channel.
				return
			}

			// Use wsjson.Write to send the JSON message.
			// We must re-create the context with a write deadline.
			writeCtx, cancel := context.WithTimeout(c.ctx, writeWait)

			// wsjson.Write handles marshaling, but our message is already marshaled.
			// A small inefficiency: it will be unmarshaled to `any` and remarshaled.
			// For raw bytes, we'd use conn.Writer and set message type.
			// Given our protocol is all JSON, wsjson.Write is safer.

			var v interface{}
			json.Unmarshal(message, &v)

			err := wsjson.Write(writeCtx, c.conn, v)
			cancel() // Release the timeout context
			if err != nil {
				log.Printf("Write error: %v", err)
				return
			}

		case <-c.ctx.Done():
			// The connection's master context was canceled.
			return
		}
	}
}

// writePump for HardwareConnection (identical logic)
func (h *HardwareConnection) writePump() {
	defer func() {
		h.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		select {
		case message, ok := <-h.send:
			if !ok {
				return
			}

			writeCtx, cancel := context.WithTimeout(h.ctx, writeWait)
			var v interface{}
			json.Unmarshal(message, &v) // Assumes payload is valid JSON

			err := wsjson.Write(writeCtx, h.conn, v)
			cancel()
			if err != nil {
				log.Printf("Write error: %v", err)
				return
			}

		case <-h.ctx.Done():
			return
		}
	}
}
