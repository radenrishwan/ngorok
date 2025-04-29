package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/radenrishwan/ngorok"
)

var (
	localPort    = flag.String("local-port", "", "Port for the local server")
	tunnelServer = flag.String("server", "", "Address of the tunnel server")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "go run main.go -local-port=8080 -server=localhost:9000\n")
	}

	flag.Parse()

	if *localPort == "" {
		flag.Usage()
		return
	}

	if *tunnelServer == "" {
		flag.Usage()
		return
	}

	fmt.Printf("Starting tunnel client to forward to localhost:%s\n", *localPort)
	fmt.Printf("Connecting to tunnel server at %s\n", *tunnelServer)

	conn, err := net.Dial("tcp", *tunnelServer)
	if err != nil {
		log.Fatalf("Failed to connect to tunnel server: %v", err)
	}

	defer conn.Close()

	decoder := json.NewDecoder(conn)
	var tunnelMsg ngorok.TunnelMessage
	if err := decoder.Decode(&tunnelMsg); err != nil {
		log.Fatalf("Failed to receive tunnel info: %v", err)
	}

	if tunnelMsg.Type != ngorok.TunnelCreated {
		log.Fatalf("Expected tunnel_created message, got %d", tunnelMsg.Type)
	}

	localUrl := tunnelMsg.Headers["Local-URL"]
	prodUrl := tunnelMsg.Headers["Prod-URL"]

	fmt.Println("\n✅ Tunnel established!")
	fmt.Printf("🏠 Local Testing URL: %s\n", localUrl)
	fmt.Printf("🌐 Production URL would be: %s\n", prodUrl)
	fmt.Printf("📡 Forwarding traffic to http://localhost:%s\n\n", *localPort)

	handleTunnelRequests(conn, *localPort)
}

func handleTunnelRequests(tunnel net.Conn, localPort string) {
	decoder := json.NewDecoder(tunnel)
	for {
		var msg ngorok.TunnelMessage
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				log.Println("Tunnel connection closed by server")
			} else {
				log.Printf("Error decoding message: %v", err)
			}
			break
		}

		if msg.Type == ngorok.TunnelRequest {
			go handleLocalRequest(tunnel, msg, localPort)
		}
	}
}

func handleLocalRequest(tunnel net.Conn, msg ngorok.TunnelMessage, localPort string) {
	fmt.Printf("↘️ %s %s\n", msg.Method, msg.Path)

	url := fmt.Sprintf("http://localhost:%s%s", localPort, msg.Path)
	req, err := http.NewRequest(msg.Method, url, strings.NewReader(msg.Body))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		sendErrorResponse(tunnel, msg.ID)
		return
	}

	for key, value := range msg.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send request to local server: %v", err)
		sendErrorResponse(tunnel, msg.ID)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		sendErrorResponse(tunnel, msg.ID)
		return
	}

	fmt.Printf("↖️ %d %s [%d bytes]\n", resp.StatusCode, msg.Path, len(body))

	headers := make(map[string]string)
	for key, values := range resp.Header {
		headers[key] = values[0]
	}
	headers["X-Status-Code"] = strconv.Itoa(resp.StatusCode)

	responseMsg := ngorok.TunnelMessage{
		Type:    ngorok.TunnelResponse,
		ID:      msg.ID,
		Headers: headers,
		Body:    string(body),
	}

	encoder := json.NewEncoder(tunnel)
	if err := encoder.Encode(responseMsg); err != nil {
		log.Printf("Failed to send response through tunnel: %v", err)
	}
}

// send error response when local service request fails
func sendErrorResponse(tunnel net.Conn, requestID string) {
	responseMsg := ngorok.TunnelMessage{
		Type: ngorok.TunnelResponse,
		ID:   requestID,
		Headers: map[string]string{
			"X-Status-Code": "502",
			"Content-Type":  "text/plain",
		},
		Body: "Bad Gateway: Unable to reach local service",
	}

	encoder := json.NewEncoder(tunnel)
	if err := encoder.Encode(responseMsg); err != nil {
		log.Printf("Failed to send error response: %v", err)
	}
}
