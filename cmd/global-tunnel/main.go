// Global Tunnel – standalone client. No Node.js required.
// Connects to a fixed tunnel server and exposes a local port with a public URL.
//
// Usage:
//
//	global-tunnel --port 3000
//	global-tunnel --port 5173 --server wss://tunnel.rahmatzadeh.com
//	global-tunnel --port 3000 --subdomain myapp
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

	port := flag.Int("port", 3000, "Local port to forward")
	server := flag.String("server", "wss://tunnel.rahmatzadeh.com", "Tunnel server URL")
	subdomain := flag.String("subdomain", "", "Optional fixed subdomain")
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

	run(*port, *server, *subdomain)
}

func run(port int, serverURL, subdomain string) {
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

	for {
		if err := connect(wsURL, port, subdomain); err != nil {
			log.Printf("Error: %v", err)
		}
		log.Printf("Disconnected. Reconnecting in 2s...")
		time.Sleep(2 * time.Second)
	}
}

func connect(wsURL string, localPort int, subdomain string) error {
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
	if err := conn.WriteJSON(reg); err != nil {
		return err
	}

	// Handle messages
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg struct {
			Type    string            `json:"type"`
			URL     string            `json:"url"`
			ID      string            `json:"id"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
			Body    string            `json:"body"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("Invalid JSON: %v", err)
			continue
		}

		switch msg.Type {
		case "registered":
			fmt.Println("\n  Tunnel is live.\n")
			fmt.Println("  Public URL:  " + msg.URL)
			fmt.Printf("  Forwarding: 127.0.0.1:%d -> %s\n", localPort, msg.URL)
			fmt.Println("\n  Press Ctrl+C to stop.\n")
		case "request":
			go handleRequest(conn, msg.ID, msg.Method, msg.URL, msg.Headers, msg.Body, localPort)
		}
	}
}

func handleRequest(conn *websocket.Conn, id, method, path string, headers map[string]string, bodyBase64 string, localPort int) {
	var body []byte
	if bodyBase64 != "" {
		body, _ = base64.StdEncoding.DecodeString(bodyBase64)
	}

	localURL := fmt.Sprintf("http://127.0.0.1:%d%s", localPort, path)
	req, err := http.NewRequest(method, localURL, nil)
	if err != nil {
		sendResponse(conn, id, 502, nil, []byte("Bad Gateway: "+err.Error()))
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
		sendResponse(conn, id, 502, nil, []byte("Bad Gateway: "+err.Error()))
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
	sendResponse(conn, id, resp.StatusCode, outHeaders, respBody)
}

func sendResponse(conn *websocket.Conn, id string, status int, headers map[string]string, body []byte) {
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
	conn.WriteJSON(payload)
}
