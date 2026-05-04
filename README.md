# gotunnel

A lightweight local development tunnel that exposes your local services to the internet.

Like ngrok, but simpler, open-source, and self-hosted.

## Features

- Single connection multiplexing — one TCP connection carries all control messages and data
- Multi-tunnel — expose multiple local services through one client
- Token authentication — prevent unauthorized access
- TLS encryption — secure the control channel
- Connection limits — protect against resource exhaustion
- Heartbeat timeout — detect dead clients automatically
- Auto-reconnect — client reconnects with exponential backoff
- Self-signed TLS — auto-generate certificates for quick setup
- HTTP mode — inject X-Forwarded-For / X-Real-IP headers automatically
- Web dashboard — real-time monitoring at a glance
- YAML config file — `--config` flag for production setups
- Docker support — ready-to-deploy Dockerfile and docker-compose

## Quick Start

### 1. Build

```bash
make build
```

### 2. Run Server (on your VPS)

```bash
./bin/gotunnel-server --port 7000
```

### 3. Run Client (on your local machine)

```bash
./bin/gotunnel-client --server your-vps:7000 --local 3000
```

Your local service on port 3000 is now accessible at `your-vps:<assigned-port>`.

### Docker

```bash
# Build and run server
docker compose up -d server

# Or build manually
docker build -t gotunnel .
docker run -p 7000:7000 gotunnel --port 7000 --token mysecret
```

## Usage

### Server

```bash
gotunnel-server [flags]

Flags:
  -p, --port int              Control channel port (default 7000)
      --min-port int          Minimum allocatable port (default 8000)
      --max-port int          Maximum allocatable port (default 9000)
      --token string          Authentication token (empty = no auth)
      --tls-cert string       Path to TLS certificate file
      --tls-key string        Path to TLS key file
      --tls-auto              Auto-generate self-signed TLS certificate
      --max-clients int       Max concurrent clients (0=unlimited)
      --max-tunnels int       Max tunnels per client (0=unlimited)
      --max-sessions int      Max concurrent sessions (0=unlimited)
      --client-timeout duration  Client heartbeat timeout, e.g. 90s (0=no timeout)
      --dashboard-port int    Web dashboard port (0=disabled)
      --config string         Path to YAML config file
```

### Client

```bash
gotunnel-client [flags]

Flags:
  -s, --server string     Server address (default "localhost:7000")
      --tunnel strings    Tunnel spec localPort:remotePort (repeatable)
  -l, --local int         Local port to expose (shorthand for --tunnel)
  -r, --remote int        Requested remote port (used with --local)
      --token string      Authentication token
      --tls               Enable TLS for control channel
      --insecure          Skip TLS certificate verification
      --http              Enable HTTP header injection (X-Forwarded-For, X-Real-IP)
      --config string     Path to YAML config file
```

## Examples

### Basic: Expose a local web server

```bash
# Server
gotunnel-server --port 7000

# Client
gotunnel-client --server vps:7000 --local 3000
```

### Multiple tunnels

Expose a web server and a database through one client:

```bash
gotunnel-client --server vps:7000 \
  --tunnel 3000 \
  --tunnel 5432:9000
```

- Port 3000 gets a random public port
- Port 5432 is mapped to public port 9000

### With authentication

```bash
# Server
gotunnel-server --port 7000 --token my-secret-token

# Client
gotunnel-client --server vps:7000 --local 3000 --token my-secret-token
```

### With TLS (self-signed)

```bash
# Server — auto-generate a self-signed certificate
gotunnel-server --port 7000 --tls-auto

# Client — connect with TLS, skip cert verification
gotunnel-client --server vps:7000 --local 3000 --tls --insecure
```

### HTTP mode (for web development)

Automatically inject `X-Forwarded-For` and `X-Real-IP` headers:

```bash
gotunnel-client --server vps:7000 --tunnel 3000 --http
```

### Web dashboard

Monitor active tunnels, connections, and traffic:

```bash
gotunnel-server --port 7000 --dashboard-port 8080
# Open http://your-vps:8080 in browser
```

### YAML config file

```bash
gotunnel-server --config server.yaml
gotunnel-client --config client.yaml
```

See `examples/` for sample config files.

### Full production setup

```bash
gotunnel-server --port 7000 \
  --token $TOKEN \
  --tls-auto \
  --max-clients 50 \
  --max-tunnels 5 \
  --max-sessions 500 \
  --client-timeout 90s \
  --min-port 10000 \
  --max-port 20000 \
  --dashboard-port 8080
```

## How It Works

```
[User] -> [VPS:public-port] -> [tunnel] -> [localhost:3000]
```

1. Client connects to server via a persistent TCP connection
2. Client sends AUTH token (if configured)
3. Client registers one or more local ports (REGISTER)
4. Server allocates public ports and starts listening
5. When someone connects to a public port, server sends NEW_CONN to client
6. Client dials the corresponding local service and relays data bidirectionally
7. Connection closes when either side disconnects

Single connection multiplexing: control messages and data share one TCP connection, distinguished by message type and connection ID.

## Development

```bash
# Run all tests
make test

# Build binaries
make build

# Clean build artifacts
make clean
```

## License

MIT
