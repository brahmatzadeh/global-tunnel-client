# Global Tunnel Client

Standalone client that exposes a local port through a public URL via a tunnel server.

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
   go build -o global-tunnel ./cmd/global-tunnel/
   ```

3. *(Optional)* Put the binary on your `PATH`:

   ```bash
   # macOS / Linux
   sudo mv global-tunnel /usr/local/bin/
   ```

   Or add the project directory to your `PATH` so you can run `./global-tunnel` from the project root.

### Cross-compile for other platforms

To build binaries for Linux, macOS (Intel/Apple Silicon), and Windows:

```bash
make build-all
```

This produces:

- `global-tunnel` — current platform
- `global-tunnel-linux-amd64`
- `global-tunnel-darwin-amd64`
- `global-tunnel-darwin-arm64`
- `global-tunnel-windows-amd64.exe`

## Usage

Expose a local port (default `3000`):

```bash
global-tunnel --port 3000
```

Options:

| Flag / Env           | Default                      | Description              |
|----------------------|------------------------------|--------------------------|
| `--port` / `TUNNEL_PORT` | `3000`                   | Local port to forward    |
| `--server` / `TUNNEL_SERVER` | `wss://tunnel.rahmatzadeh.com` | Tunnel server URL |
| `--subdomain` / `TUNNEL_SUBDOMAIN` | *(none)*           | Optional fixed subdomain |

Examples:

```bash
# Forward port 5173 (e.g. Vite dev server)
global-tunnel --port 5173

# Use a custom tunnel server
global-tunnel --port 3000 --server wss://tunnel.example.com

# Request a fixed subdomain
global-tunnel --port 3000 --subdomain myapp
```

When connected, the client prints the public URL. Press Ctrl+C to stop.
