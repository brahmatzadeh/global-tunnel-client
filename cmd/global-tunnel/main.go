// Global Tunnel – standalone client. No Node.js required.
// Connects to a fixed tunnel server and exposes a local port with a public URL.
// Optional TCP tunnel: forward raw TCP (e.g. Postgres) via server's TCP port.
//
// Usage:
//
//	gtc --port 3000
//	gtc --port 5173 --server wss://tunnel.rahmatzadeh.com
//	gtc --port 3000 --subdomain myapp
//	gtc --port 3000 --subdomain db --tcp-port 5432
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	// Support --flag (rewrite to -flag for Go's flag package)
	for i, a := range os.Args {
		if strings.HasPrefix(a, "--") && len(a) > 2 {
			os.Args[i] = "-" + a[2:]
		}
	}

	port := flag.Int("port", 3000, "Local port to forward (HTTP)")
	server := flag.String("server", "wss://tunnel.rahmatzadeh.com", "Tunnel server URL")
	subdomain := flag.String("subdomain", "", "Optional fixed subdomain")
	tcpPort := flag.Int("tcp-port", 0, "Local port to forward for TCP tunnel (1-65535); 0 = off")
	flag.Parse()

	if p := os.Getenv("TUNNEL_PORT"); p != "" {
		fmt.Sscanf(p, "%d", port)
	}
	if s := os.Getenv("TUNNEL_SERVER"); s != "" {
		*server = s
	}
	if s := os.Getenv("TUNNEL_SUBDOMAIN"); s != "" {
		*subdomain = s
	}
	if p := os.Getenv("TUNNEL_TCP_PORT"); p != "" {
		fmt.Sscanf(p, "%d", tcpPort)
	}

	run(*port, *server, *subdomain, *tcpPort)
}

func run(port int, serverURL, subdomain string, tcpPort int) {
	wsURL := serverURL
	wsURL = strings.TrimSuffix(wsURL, "/")
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + wsURL[8:]
	} else if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + wsURL[7:]
	}
	wsURL += "/_tunnel"

	log.Printf("Connecting to tunnel server at %s", serverURL)
	log.Printf("Local port: %d", port)
	if tcpPort > 0 {
		log.Printf("TCP tunnel: local port %d", tcpPort)
	}

	for {
		if err := connect(wsURL, serverURL, port, subdomain, tcpPort); err != nil {
			log.Printf("Error: %v", err)
		}
		log.Printf("Disconnected. Reconnecting in 2s...")
		time.Sleep(2 * time.Second)
	}
}

func connect(wsURL, serverURL string, localPort int, subdomain string, tcpPort int) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Register
	reg := map[string]interface{}{"type": "register"}
	if subdomain != "" {
		reg["subdomain"] = subdomain
	}
	if tcpPort >= 1 && tcpPort <= 65535 {
		reg["tcpPort"] = tcpPort
	}
	if err := conn.WriteJSON(reg); err != nil {
		return err
	}

	var tcpConns sync.Map // id -> *tcpConnState
	var wsWriteMu sync.Mutex
	writeWS := func(v interface{}) {
		wsWriteMu.Lock()
		defer wsWriteMu.Unlock()
		conn.WriteJSON(v)
	}

	// Handle messages
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg struct {
			Type                   string            `json:"type"`
			URL                    string            `json:"url"`
			Subdomain              string            `json:"subdomain"`
			RequestedSubdomain     string            `json:"requestedSubdomain"`
			UsedRequestedSubdomain bool              `json:"usedRequestedSubdomain"`
			TcpTunnelPort          int               `json:"tcpTunnelPort"`
			ID                     string            `json:"id"`
			Port                   int               `json:"port"`
			Payload                string            `json:"payload"`
			Method                 string            `json:"method"`
			Headers                map[string]string `json:"headers"`
			Body                   string            `json:"body"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("Invalid JSON: %v", err)
			continue
		}

		switch msg.Type {
		case "registered":
			fmt.Println("\n  Tunnel is live.\n")
			fmt.Println("  Public URL:  " + msg.URL)
			if msg.Subdomain != "" {
				fmt.Printf("  Subdomain:   %s\n", msg.Subdomain)
			}
			if msg.RequestedSubdomain != "" && !msg.UsedRequestedSubdomain {
				fmt.Printf("  Note: requested subdomain %q was taken or invalid; assigned %q instead.\n", msg.RequestedSubdomain, msg.Subdomain)
			}
			fmt.Printf("  Forwarding: 127.0.0.1:%d -> %s\n", localPort, msg.URL)
			if msg.TcpTunnelPort > 0 {
				host := serverHost(serverURL)
				fmt.Printf("  TCP tunnel:  connect to %s:%d, send subdomain line then raw bytes (e.g. echo %q | nc %s %d)\n", host, msg.TcpTunnelPort, msg.Subdomain, host, msg.TcpTunnelPort)
			}
			fmt.Println("\n  Press Ctrl+C to stop.\n")
		case "request":
			go handleRequest(conn, &wsWriteMu, msg.ID, msg.Method, msg.URL, msg.Headers, msg.Body, localPort)
		case "tcp-connect":
			go handleTcpConnect(writeWS, &tcpConns, msg.ID, msg.Port)
		case "tcp-data":
			if v, ok := tcpConns.Load(msg.ID); ok {
				if state := v.(*tcpConnState); state.conn != nil {
					payload, _ := base64.StdEncoding.DecodeString(msg.Payload)
					if _, err := state.conn.Write(payload); err != nil {
						tcpCloseAndCleanup(writeWS, &tcpConns, msg.ID)
					}
				}
			}
		case "tcp-close":
			tcpCleanupOnly(&tcpConns, msg.ID)
		}
	}
}

func serverHost(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "server"
	}
	if u.Host != "" {
		return u.Hostname()
	}
	return "server"
}

type tcpConnState struct {
	conn net.Conn
	once sync.Once
}

func (s *tcpConnState) close() {
	s.once.Do(func() {
		if s.conn != nil {
			s.conn.Close()
		}
	})
}

func handleTcpConnect(writeWS func(interface{}), tcpConns *sync.Map, id string, localPort int) {
	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		writeWS(map[string]interface{}{"type": "tcp-error", "id": id})
		return
	}
	state := &tcpConnState{conn: conn}
	tcpConns.Store(id, state)
	writeWS(map[string]interface{}{"type": "tcp-connected", "id": id})
	// Copy local -> WebSocket
	go func() {
		defer tcpCloseAndCleanup(writeWS, tcpConns, id)
		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				writeWS(map[string]interface{}{
					"type":    "tcp-data",
					"id":     id,
					"payload": base64.StdEncoding.EncodeToString(buf[:n]),
				})
			}
			if err != nil {
				return
			}
		}
	}()
}

func tcpCleanupOnly(tcpConns *sync.Map, id string) {
	if v, loaded := tcpConns.LoadAndDelete(id); loaded {
		if state, ok := v.(*tcpConnState); ok {
			state.close()
		}
	}
}

func tcpCloseAndCleanup(writeWS func(interface{}), tcpConns *sync.Map, id string) {
	if v, loaded := tcpConns.LoadAndDelete(id); loaded {
		if state, ok := v.(*tcpConnState); ok {
			state.close()
		}
		writeWS(map[string]interface{}{"type": "tcp-close", "id": id})
	}
}

func handleRequest(conn *websocket.Conn, wsWriteMu *sync.Mutex, id, method, path string, headers map[string]string, bodyBase64 string, localPort int) {
	var body []byte
	if bodyBase64 != "" {
		body, _ = base64.StdEncoding.DecodeString(bodyBase64)
	}

	localURL := fmt.Sprintf("http://127.0.0.1:%d%s", localPort, path)
	req, err := http.NewRequest(method, localURL, nil)
	if err != nil {
		sendResponse(conn, wsWriteMu, id, 502, nil, []byte("Bad Gateway: "+err.Error()))
		return
	}
	if body != nil {
		req.Body = io.NopCloser(strings.NewReader(string(body)))
		req.ContentLength = int64(len(body))
	}

	req.Host = fmt.Sprintf("127.0.0.1:%d", localPort)
	for k, v := range headers {
		kl := strings.ToLower(k)
		if kl == "host" || kl == "connection" || kl == "keep-alive" {
			continue
		}
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 55 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		sendResponse(conn, wsWriteMu, id, 502, nil, []byte("Bad Gateway: "+err.Error()))
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	outHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			outHeaders[k] = strings.Join(v, ", ")
		}
	}
	sendResponse(conn, wsWriteMu, id, resp.StatusCode, outHeaders, respBody)
}

func sendResponse(conn *websocket.Conn, wsWriteMu *sync.Mutex, id string, status int, headers map[string]string, body []byte) {
	if headers == nil {
		headers = map[string]string{"Content-Type": "text/plain"}
	}
	payload := map[string]interface{}{
		"type":   "response",
		"id":     id,
		"status": status,
		"headers": headers,
		"body":   base64.StdEncoding.EncodeToString(body),
	}
	wsWriteMu.Lock()
	conn.WriteJSON(payload)
	wsWriteMu.Unlock()
}
