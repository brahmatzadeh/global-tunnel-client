# Global Tunnel Client

Standalone client that exposes a local port through a public URL via a tunnel server.

**Server:** [global-tunnel-server](https://github.com/brahmatzadeh/global-tunnel-server)

## Prerequisites

- **Go 1.21 or later** — [Install Go](https://go.dev/doc/install)

## Installation

### From source

1. Clone the repository:

   ```bash
   git clone https://github.com/your-org/global-tunnel-client.git
   cd global-tunnel-client
   ```

2. Build the binary:

   ```bash
   make build
   ```

   Or without Make:

   ```bash
   go build -o gtc ./cmd/global-tunnel/
   ```

3. *(Optional)* Put the binary on your `PATH`:

   ```bash
   # macOS / Linux
   sudo mv gtc /usr/local/bin/
   ```

   Or add the project directory to your `PATH` so you can run `./gtc` from the project root.

### Cross-compile for other platforms

To build binaries for Linux, macOS (Intel/Apple Silicon), and Windows:

```bash
make build-all
```

This produces:

- `gtc` — current platform
- `gtc-linux-amd64`
- `gtc-darwin-amd64`
- `gtc-darwin-arm64`
- `gtc-windows-amd64.exe`

## Usage

Expose a local port (default `3000`):

```bash
gtc --port 3000
```

Options:

| Flag / Env           | Default                      | Description              |
|----------------------|------------------------------|--------------------------|
| `--port` / `TUNNEL_PORT` | `3000`                   | Local port to forward (HTTP) |
| `--server` / `TUNNEL_SERVER` | `wss://tunnel.rahmatzadeh.com` | Tunnel server URL |
| `--subdomain` / `TUNNEL_SUBDOMAIN` | *(none)*           | Optional fixed subdomain |
| `--tcp-port` / `TUNNEL_TCP_PORT` | `0` (off)            | Local port to forward as raw TCP (e.g. 5432 for Postgres); requires server TCP tunnel enabled |

Examples:

```bash
# Forward port 5173 (e.g. Vite dev server)
gtc --port 5173

# Use a custom tunnel server
gtc --port 3000 --server wss://tunnel.example.com

# Request a fixed subdomain
gtc --port 3000 --subdomain myapp

# HTTP + TCP tunnel (e.g. Postgres on 5432)
gtc --port 3000 --subdomain db --tcp-port 5432
```

When connected, the client prints the public URL (and, if TCP is enabled, the server’s TCP port and how to connect). Press Ctrl+C to stop.

### TCP tunnel

If the server has TCP tunneling enabled and you set `--tcp-port` (e.g. `5432`), the client registers that port. The server responds with `tcpTunnelPort` (e.g. `4000`). To connect from a public client:

1. Connect to the server’s TCP port (e.g. `server:4000`).
2. Send one line: `subdomain\n` (e.g. `db\n`).
3. Then send or receive raw TCP bytes (e.g. Postgres protocol).

Example: `echo "db" | nc server 4000` then type or pipe data; the stream is forwarded to the client’s `localhost:5432`.