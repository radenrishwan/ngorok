package ngorok

// Tunnel Message type
const (
	TunnelCreated = iota
	TunnelDestroyed

	TunnelRequest
	TunnelResponse
)

// Used to communicate between the client and the server
type TunnelMessage struct {
	Type    int               `json:"type"`
	Method  string            `json:"method"`
	ID      string            `json:"id"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}
