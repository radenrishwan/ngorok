package ngorok

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ServerOption struct {
	Timeout time.Duration
}

var DefaultServerOption = ServerOption{
	Timeout: 30 * time.Second,
}

type Server struct {
	port       string
	baseDomain string
	option     ServerOption
}

func NewServer(port string, baseDomain string, option *ServerOption) *Server {
	if option == nil {
		option = &DefaultServerOption
	}

	return &Server{
		port:       port,
		baseDomain: baseDomain,
		option:     *option,
	}
}

func (self *Server) Start(errMessage chan error) error {
	http.HandleFunc("/", self.handlePublicRequest)

	http.HandleFunc("/hc", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	return http.ListenAndServe(":"+self.port, nil)
}

func (self *Server) handlePublicRequest(w http.ResponseWriter, r *http.Request) {
	var tunnelID string
	var requestPath string

	// handle local testing
	if strings.HasPrefix(r.URL.Path, "/tunnel/") {
		parts := strings.SplitN(r.URL.Path[8:], "/", 2)
		tunnelID = parts[0]
		if len(parts) > 1 {
			requestPath = "/" + parts[1]
		} else {
			requestPath = "/"
		}
	} else {
		host := r.Host
		if strings.HasSuffix(host, "."+self.baseDomain) {
			tunnelID = strings.TrimSuffix(host, "."+self.baseDomain)
			requestPath = r.URL.Path
		} else {
			http.Error(w, "Invalid tunnel URL", http.StatusBadRequest)
			return
		}
	}

	if tunnelID == "" {
		w.Write([]byte("Welcome to Ngorok, the tunnel service."))
		return
	}

	conn, exists := tunnels.Load(tunnelID)
	if !exists {
		http.Error(w, "Tunnel not found or has expired", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}

	headers := make(map[string]string)
	for key, values := range r.Header {
		headers[key] = values[0]
	}

	headers["X-Forwarded-Host"] = r.Host
	headers["X-Forwarded-Proto"] = "http"

	requestID := fmt.Sprintf("%s-%d", tunnelID, time.Now().UnixNano())

	// create response channel
	responseCh := make(chan TunnelMessage)
	pendingRequests.Store(requestID, responseCh)

	msg := TunnelMessage{
		Type:    TunnelRequest,
		ID:      requestID,
		Method:  r.Method,
		Path:    requestPath,
		Headers: headers,
		Body:    string(body),
	}

	encoder := json.NewEncoder(conn.(net.Conn))
	if err := encoder.Encode(msg); err != nil {
		http.Error(w, "Failed to send request through tunnel", http.StatusInternalServerError)
		pendingRequests.Delete(requestID)
		return
	}

	select {
	case response := <-responseCh:
		for key, value := range response.Headers {
			if key != "X-Status-Code" {
				w.Header().Set(key, value)
			}
		}

		if statusCode, ok := response.Headers["X-Status-Code"]; ok {
			code, err := strconv.Atoi(statusCode)
			if err == nil && code > 0 {
				w.WriteHeader(code)
			}
		}

		w.Write([]byte(response.Body))
	case <-time.After(self.option.Timeout):
		http.Error(w, "Request timed out", http.StatusGatewayTimeout)
	}
}
