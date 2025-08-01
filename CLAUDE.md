# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

High-performance QUIC-based gateway server that bridges QUIC clients with gRPC upstream services. The system provides real-time communication with session management, message reliability, and comprehensive performance monitoring.

## Common Commands

### Building and Setup
```bash
# Complete build (dependencies, protobuf, certificates, binaries)
make all

# Build only binaries
make build

# Build development tools (monitor, gencerts)
make tools

# Generate TLS certificates
make certs

# Generate protobuf code
make proto

# Install Go dependencies
make deps
```

### Running Services
```bash
# Start upstream service (run first)
make run-upstream

# Start gateway server (run second, in new terminal)
make run-gatesvr

# Start client in interactive mode (run third, in new terminal)
make run-client

# Alternative manual startup
./bin/upstream.exe -addr :9000
./bin/gatesvr.exe -quic :8443 -http :8080 -upstream localhost:9000
./bin/client.exe -server localhost:8443 -interactive
```

### Testing and Monitoring
```bash
# Run Go tests
make test

# Performance testing
make perf-test
make load-test          # 5 concurrent clients
make continuous-test    # Single client continuous

# Performance monitoring
make monitor

# Code formatting and linting
make fmt
make lint
```

### Development
```bash
# Clean build artifacts
make clean

# Development mode with file watching (requires air tool)
make dev

# Quick test (automated startup and testing)
make quick-test
```

## Architecture Overview

### Core Components

**Multi-Protocol Gateway Server:**
- **QUIC Server** (`:8443`) - Primary client communication protocol
- **HTTP API** (`:8080`) - Management/monitoring REST endpoints
- **gRPC Service** (`:8082`) - Upstream service callback interface  
- **Metrics Server** (`:9090`) - Prometheus metrics export

**Service Components:**
- **Gateway Server** (`cmd/gatesvr`) - Main QUIC gateway with session management
- **Upstream Service** (`cmd/upstream`) - Business logic processor with gRPC interface
- **Client** (`cmd/client`) - Test client with performance testing capabilities
- **Monitor** (`cmd/monitor`) - Real-time performance monitoring tool

### Message Flow Architecture

```
Client (QUIC) → Gateway → Upstream (gRPC)
     ↑                         ↓
     ← Session Mgmt ← Push Notifications
```

**Key Flow:**
1. Client establishes QUIC connection with session management
2. Gateway validates sessions and forwards requests to upstream via gRPC
3. Upstream processes business logic and can trigger push notifications
4. Gateway manages reliable message delivery with ACK mechanism
5. Clients support automatic reconnection with session continuity

### Session Management System

**Advanced Features:**
- **Multi-index storage** by Session ID, GID (game ID), OpenID (user ID)
- **Reconnection support** with seamless session transfer
- **Message caching** for pre-login and failed delivery scenarios
- **ACK-based reliability** with timeout and retry mechanisms
- **Session states**: `Inited` → `Normal` → `Closed`

### Performance Monitoring

**Comprehensive Latency Tracking:**
- Read, Parse, Upstream, Encode, Send, and Total latency measurements
- P95/P99 percentile calculations with real-time QPS monitoring
- HTTP endpoints: `/health`, `/stats`, `/performance`
- Prometheus metrics integration at `:9090/metrics`

## Key Internal Packages

- **`internal/gateway/`** - Core gateway logic, multi-protocol server, session management
- **`internal/session/`** - Advanced session lifecycle, multi-index storage, reconnection handling
- **`internal/client/`** - Smart client with reconnection, latency tracking, response correlation
- **`internal/upstream/`** - Upstream service logic and unicast client for push notifications
- **`internal/message/`** - Protocol buffer codec and message validation
- **`pkg/metrics/`** - Performance metrics collection and analysis

## Protocol Design

**Two-Layer Protocol:**
- **Transport Layer** (`proto/message.proto`) - Generic message envelopes with reliability
- **Business Layer** (`proto/upstream.proto`) - Business logic interface

**Message Types:**
- **Client Requests**: START, STOP, HEARTBEAT, BUSINESS, ACK
- **Server Push**: START_RESP, HEARTBEAT_RESP, BUSINESS_DATA, ERROR

## Startup Sequence

**Critical Order:**
1. Start upstream service (`:9000`)
2. Wait 3-5 seconds for full initialization
3. Start gateway server (`:8443`, `:8080`, `:9090`)
4. Wait 3-5 seconds for session manager initialization
5. Start clients for testing

## Testing Approach

**Test Framework:** Standard Go testing with `go test ./...`

**Client Test Modes:**
- **Interactive mode** (`-interactive`) - Manual testing interface
- **Performance mode** (`-performance`) - Load testing with multiple clients
- **Continuous mode** (default) - Long-running connection testing
- **Single test mode** (`-test <type>`) - One-shot functionality testing

**Performance Testing:**
```bash
# Multi-client load test
./bin/client.exe -server localhost:8443 -performance -clients 5 -request-interval 200ms

# Continuous monitoring during test
./bin/monitor.exe -server localhost:8080 -continuous -interval 2s
```

## Configuration

**Environment Variables:**
- Server ports, upstream addresses, and monitoring intervals configurable via CLI flags
- TLS certificates auto-generated in `certs/` directory
- All components support graceful shutdown via SIGINT/SIGTERM

**Key Files:**
- `Makefile` - Complete build and operational commands
- `QUICKSTART.md` - Detailed usage instructions (Chinese)
- `go.mod` - Go 1.23+ with QUIC, gRPC, and Prometheus dependencies