# gotunnel

A lightweight local development tunnel that exposes your local services to the internet.

Like ngrok, but simpler, open-source, and self-hosted.

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

## Usage

### Server

```bash
gotunnel-server [flags]

Flags:
  -p, --port int       Control channel port (default 7000)
      --min-port int   Minimum allocatable port (default 8000)
      --max-port int   Maximum allocatable port (default 9000)
```

### Client

```bash
gotunnel-client [flags]

Flags:
  -s, --server string   Server address (default "localhost:7000")
  -l, --local int       Local port to expose (default 3000)
  -r, --remote int      Requested remote port, 0 for auto (default 0)
```

## How It Works

1. Client connects to server via a persistent TCP connection
2. Client registers which local port to expose
3. Server allocates a public port and starts listening
4. When someone connects to the public port, data is relayed through the tunnel to your local service

## Architecture

```
[User] -> [VPS:public-port] -> [tunnel] -> [localhost:3000]
```

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
