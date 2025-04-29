package ngorok

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

var (
	tunnels         = sync.Map{}
	pendingRequests = sync.Map{}

	generator = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func generateTunnelID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, 8) // 8-character ID
	for i := range result {
		result[i] = chars[generator.Intn(len(chars))]
	}
	return string(result)
}

type TunnelServerOption struct {
	OnRequest func(id string, conn net.Conn)
	HttpPort  string
}

var DefaultTunnelServerOption = TunnelServerOption{
	OnRequest: func(id string, conn net.Conn) {
		fmt.Printf("New tunnel established: %s\n", id)
	},
}

type TunnelServer struct {
	port       string
	baseDomain string
	option     TunnelServerOption
}

func NewTunnelServer(port string, baseDomain string, option *TunnelServerOption) *TunnelServer {
	if option == nil {
		option = &DefaultTunnelServerOption
	}

	return &TunnelServer{
		port:       port,
		baseDomain: baseDomain,
		option:     *option,
	}
}

// start tunnel server,
func (self *TunnelServer) Start(errMessage chan error) error {
	listener, err := net.Listen("tcp", ":"+self.port)
	if err != nil {
		return err
	}

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			errMessage <- err
			continue
		}

		tunnelId := generateTunnelID()

		self.option.OnRequest(tunnelId, conn)

		go self.handleTunnelConnection(tunnelId, conn, errMessage)
	}
}

func (self *TunnelServer) handleTunnelConnection(id string, conn net.Conn, errMessage chan error) {
	defer conn.Close()

	tunnels.Store(id, conn)

	localURL := fmt.Sprintf("http://localhost:%s/tunnel/%s", self.option.HttpPort, id)
	prodURL := fmt.Sprintf("http://%s.%s", id, self.baseDomain)

	msg := TunnelMessage{
		Type: TunnelCreated,
		ID:   id,
		Headers: map[string]string{
			"Local-URL": localURL,
			"Prod-URL":  prodURL,
		},
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(msg); err != nil {
		log.Printf("Failed to send tunnel info: %v", err)
		return
	}

	// clean up the tunnel
	defer func() {
		conn.Close()
		tunnels.Delete(id)
	}()

	// handle tunnel requests
	decoder := json.NewDecoder(conn)
	for {
		var msg TunnelMessage
		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF {
				errMessage <- err
			}

			break
		}

		// delete request from queue
		if msg.Type == TunnelResponse {
			if ch, exists := pendingRequests.Load(msg.ID); exists {
				ch.(chan TunnelMessage) <- msg
				pendingRequests.Delete(msg.ID)
			}
		}
	}
}
