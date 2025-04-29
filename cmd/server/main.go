package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/radenrishwan/ngorok"
)

// go run main.go -tunnel-port=9001 -http-port=8080 -domain=example.com
var (
	tunnelPort = flag.String("tunnel-port", "9000", "Port for the tunnel server")
	httpPort   = flag.String("http-port", "8000", "Port for the HTTP server")
	baseDomain = flag.String("domain", "localhost", "Base domain for tunnels")
	enableTLS  = flag.Bool("tls", false, "Enalbe TLS with Let's Encrypt")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "go run main.go -tunnel-port=9001 -http-port=8080 -domain=example.com\n")
	}

	flag.Parse()

	tunnelPort := *tunnelPort
	httpPort := *httpPort
	baseDomain := *baseDomain
	enableTLS := *enableTLS

	fmt.Println("Starting tunnel server...")

	tunnelServer := ngorok.NewTunnelServer(tunnelPort, baseDomain, &ngorok.TunnelServerOption{
		OnRequest: func(id string, conn net.Conn) {
			localURL := fmt.Sprintf("http://localhost:%s/tunnel/%s", httpPort, id)
			prodURL := fmt.Sprintf("http://%s.%s", id, baseDomain)
			fmt.Printf("New tunnel established: %s\n", id)
			fmt.Printf("🏠 %s\n", localURL)
			fmt.Printf("🌐 %s\n", prodURL)
		},
		OnTunnelDestroyed: func(id string) {
			fmt.Printf("👊 %s destroyed\n", id)
		},
		HttpPort:  httpPort,
		EnableTLS: enableTLS,
	})

	errMessage := make(chan error)

	go tunnelServer.Start(errMessage)

	fmt.Println("Starting HTTP server...")
	server := ngorok.NewServer(httpPort, baseDomain, &ngorok.ServerOption{
		Timeout: time.Second * 30,
	})
	go server.Start(errMessage)

	for {
		select {
		case err := <-errMessage:
			log.Println(err)
		}
	}
}
