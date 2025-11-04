package websocket

// ClientCallPayload is the JSON struct a client sends.
type ClientCallPayload struct {
	Command string `json:"command"`
}

// ImagePropsPayload is the JSON struct the server sends to the client
// after processing the hardware's binary image.
type ImagePropsPayload struct {
	Size int64 `json:"size"`
}

// ErrorMessage is a standard error response struct.
type ErrorMessage struct {
	Error string `json:"error"`
}

// ClientMessage is an internal struct to pass messages from a client
// to the hub's relay channel.
type ClientMessage struct {
	Client  *ClientConnection
	Payload []byte
}

// HardwareMessage is an internal struct to pass messages from hardware
// to the hub's relay channel.
type HardwareMessage struct {
	Hardware *HardwareConnection
	Payload  []byte
}
