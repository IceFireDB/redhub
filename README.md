<p align="center">
    <img 
        src="https://user-images.githubusercontent.com/12872991/134626503-c022bb8e-2d5c-4760-a470-f56ff8ef036f.png" 
        border="0" alt="REDHUB">
    <br>
</p>

# RedHub

[![GoDoc Reference](https://img.shields.io/badge/api-reference-blue.svg?style=flat-square)](https://pkg.go.dev/github.com/IceFireDB/redhub)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FIceFireDB%2Fredhub.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2FIceFireDB%2Fredhub?ref=badge_shield)
[![Go Report Card](https://goreportcard.com/badge/github.com/IceFireDB/redhub)](https://goreportcard.com/report/github.com/IceFireDB/redhub)
[![License](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)

RedHub is a high-performance RESP (Redis Serialization Protocol) server framework built in Go. It leverages the RawEpoll model via the [gnet](https://github.com/panjf2000/gnet) library to achieve ultra-high throughput with multi-threaded support while maintaining low CPU resource consumption.

## Features

- **Ultra High Performance** - Exceeds Redis single-threaded and multi-threaded implementations in benchmarks
- **Fully Multi-threaded** - Native support for multiple CPU cores with efficient event loop distribution
- **Low Resource Consumption** - Optimized memory usage and CPU efficiency
- **Full RESP Protocol Support** - Compatible with Redis protocol (RESP2)
- **Multi-Protocol Support** - Supports RESP, Tile38 native, and Telnet protocols
- **Easy to Use** - Create Redis-compatible servers with minimal code
- **Production Ready** - Robust error handling, connection management, and extensibility

## Architecture

RedHub implements an event-driven architecture based on the gnet framework:

```
┌─────────────────────────────────────────────────────────────┐
│                         Client Connections                  │
└─────────────────────────────┬───────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Event Loops (gnet)                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ Event Loop 1│  │ Event Loop 2│  │ Event Loop N│        │
│  │ (Thread 1)  │  │ (Thread 2)  │  │ (Thread N)  │        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
│         │                 │                 │               │
│         └─────────────────┼─────────────────┘               │
│                           │                                  │
│                           ▼                                  │
│                  ┌──────────────┐                           │
│                  │ RedHub Core  │                           │
│                  │   Handler    │                           │
│                  └──────┬───────┘                           │
└───────────────────────────┼──────────────────────────────────┘
                            │
                            ▼
                  ┌─────────────────┐
                  │ Application     │
                  │ Logic & Storage │
                  └─────────────────┘
```

### Threading Model

- **Single-core mode**: All connections handled by a single event loop
- **Multi-core mode**: Multiple event loops distribute connections using configurable load balancing strategies
- **Connection Buffering**: Each connection maintains its own buffer for command accumulation
- **Thread Safety**: Uses RWMutex for connection map synchronization

## Installation

```bash
go get -u github.com/IceFireDB/redhub
```

## Quick Start

Here's a simple example showing how to create a Redis-compatible server with SET, GET, DEL, PING, and QUIT commands:

### Example Code

```go
package main

import (
    "log"
    "strings"
    "sync"

    "github.com/IceFireDB/redhub"
    "github.com/IceFireDB/redhub/pkg/resp"
)

func main() {
    var mu sync.RWMutex
    var items = make(map[string][]byte)

    // Create a new RedHub instance
    rh := redhub.NewRedHub(
        // OnOpen: Called when a new connection is established
        func(c *redhub.Conn) (out []byte, action redhub.Action) {
            // Initialize connection-specific data here
            return nil, redhub.None
        },
        // OnClose: Called when a connection is closed
        func(c *redhub.Conn, err error) (action redhub.Action) {
            // Clean up connection-specific data here
            return redhub.None
        },
        // Handler: Called for each parsed command
        func(cmd resp.Command, out []byte) ([]byte, redhub.Action) {
            // Get command name (case-insensitive)
            cmdName := strings.ToLower(string(cmd.Args[0]))

            switch cmdName {
            case "set":
                // SET key value
                if len(cmd.Args) != 3 {
                    return resp.AppendError(out, 
                        "ERR wrong number of arguments for 'set' command"), redhub.None
                }
                mu.Lock()
                items[string(cmd.Args[1])] = cmd.Args[2]
                mu.Unlock()
                return resp.AppendString(out, "OK"), redhub.None

            case "get":
                // GET key
                if len(cmd.Args) != 2 {
                    return resp.AppendError(out, 
                        "ERR wrong number of arguments for 'get' command"), redhub.None
                }
                mu.RLock()
                val, ok := items[string(cmd.Args[1])]
                mu.RUnlock()
                if !ok {
                    return resp.AppendNull(out), redhub.None
                }
                return resp.AppendBulk(out, val), redhub.None

            case "del":
                // DEL key
                if len(cmd.Args) != 2 {
                    return resp.AppendError(out, 
                        "ERR wrong number of arguments for 'del' command"), redhub.None
                }
                mu.Lock()
                _, ok := items[string(cmd.Args[1])]
                delete(items, string(cmd.Args[1]))
                mu.Unlock()
                if !ok {
                    return resp.AppendInt(out, 0), redhub.None
                }
                return resp.AppendInt(out, 1), redhub.None

            case "ping":
                // PING
                return resp.AppendString(out, "PONG"), redhub.None

            case "quit":
                // QUIT
                return resp.AppendString(out, "OK"), redhub.Close

            default:
                // Unknown command
                return resp.AppendError(out, 
                    "ERR unknown command '"+string(cmd.Args[0])+"'"), redhub.None
            }
        },
    )

    // Start the server
    err := redhub.ListenAndServe("tcp://127.0.0.1:6379", redhub.Options{
        Multicore: true,  // Enable multi-core support
    }, rh)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Run the Example

```bash
# Navigate to the example directory
cd example/memory_kv

# Run the server
go run server.go

# In another terminal, test with redis-cli
redis-cli -p 6379

# Or test with redis-benchmark
redis-benchmark -h 127.0.0.1 -p 6379 -n 1000000 -t set,get -c 512 -P 1024 -q
```

## Configuration

RedHub provides various configuration options through the `Options` struct:

```go
type Options struct {
    Multicore        bool              // Enable multi-core support (default: false)
    LockOSThread     bool              // Lock OS thread (default: false)
    ReadBufferCap    int               // Read buffer capacity (default: 64KB)
    LB               gnet.LoadBalancing // Load balancing strategy (default: RoundRobin)
    NumEventLoop     int               // Number of event loops (default: runtime.NumCPU())
    ReusePort        bool              // Enable port reuse (default: false)
    Ticker           bool              // Enable ticker (default: false)
    TCPKeepAlive     time.Duration     // TCP keep-alive interval
    TCPKeepCount     int               // TCP keep-alive count
    TCPKeepInterval  time.Duration     // TCP keep-alive interval
    TCPNoDelay       gnet.TCPSocketOpt // TCP no-delay option
    SocketRecvBuffer int               // Socket receive buffer size
    SocketSendBuffer int               // Socket send buffer size
    EdgeTriggeredIO  bool              // Edge-triggered I/O (default: false)
}
```

### Example Configuration

```go
options := redhub.Options{
    Multicore:        true,              // Enable multi-core
    NumEventLoop:     8,                 // Use 8 event loops
    ReadBufferCap:    64 * 1024,         // 64KB read buffer
    SocketRecvBuffer: 128 * 1024,        // 128KB socket receive buffer
    SocketSendBuffer: 128 * 1024,        // 128KB socket send buffer
    TCPKeepAlive:     30 * time.Second,  // 30s keep-alive
    LB:               gnet.LeastConnections, // Load balancing strategy
}
```

## API Reference

### Core Types

#### Action

`Action` represents the action to take after an event handler completes.

```go
const (
    None     // No action
    Close    // Close the connection
    Shutdown // Shutdown the server
)
```

#### RedHub

`RedHub` is the main server structure that manages connections and command processing.

#### Conn

`Conn` wraps a gnet.Conn and provides additional functionality for connection management.

#### Command

`Command` represents a parsed RESP command with raw bytes and arguments.

```go
type Command struct {
    Raw  []byte   // Raw RESP message
    Args [][]byte // Parsed arguments
}
```

### Main Functions

#### NewRedHub

Creates a new RedHub instance with the specified event handlers.

```go
func NewRedHub(
    onOpened func(c *Conn) (out []byte, action Action),
    onClosed func(c *Conn, err error) (action Action),
    handler func(cmd resp.Command, out []byte) ([]byte, Action),
) *RedHub
```

**Parameters:**
- `onOpened`: Called when a new connection is established
- `onClosed`: Called when a connection is closed
- `handler`: Called for each parsed command

#### ListenAndServe

Starts the RedHub server with the specified address and options.

```go
func ListenAndServe(addr string, options Options, rh *RedHub) error
```

**Parameters:**
- `addr`: Server address in format "tcp://host:port"
- `options`: Server configuration options
- `rh`: RedHub instance

## RESP Protocol Package

The `resp` package provides comprehensive support for the Redis Serialization Protocol (RESP).

### RESP Types

```go
const (
    Integer = ':'  // Integers (e.g., :1000\r\n)
    String  = '+'  // Simple strings (e.g., +OK\r\n)
    Bulk    = '$'  // Bulk strings (e.g., $6\r\nfoobar\r\n)
    Array   = '*'  // Arrays (e.g., *2\r\n$3\r\nGET\r\n$3\r\nkey\r\n)
    Error   = '-'  // Errors (e.g., -ERR unknown command\r\n)
)
```

### RESP Serialization Functions

The `resp` package provides functions for serializing various Go types to RESP format:

- `AppendInt(b []byte, n int64) []byte` - Append integer
- `AppendString(b []byte, s string) []byte` - Append simple string
- `AppendBulk(b []byte, bulk []byte) []byte` - Append bulk bytes
- `AppendBulkString(b []byte, bulk string) []byte` - Append bulk string
- `AppendArray(b []byte, n int) []byte` - Append array header
- `AppendError(b []byte, s string) []byte` - Append error
- `AppendNull(b []byte) []byte` - Append null value
- `AppendOK(b []byte) []byte` - Append OK response
- `AppendAny(b []byte, v interface{}) []byte` - Append any Go type

### Example: Building Responses

```go
var out []byte

// Simple string
out = resp.AppendString(out, "OK")

// Bulk string
out = resp.AppendBulkString(out, "Hello World")

// Integer
out = resp.AppendInt(out, 42)

// Array
out = resp.AppendArray(out, 3)
out = resp.AppendBulkString(out, "item1")
out = resp.AppendBulkString(out, "item2")
out = resp.AppendBulkString(out, "item3")

// Error
out = resp.AppendError(out, "ERR something went wrong")

// Null value
out = resp.AppendNull(out)

// Any type
out = resp.AppendAny(out, map[string]interface{}{
    "name": "Redis",
    "version": 7.0,
    "features": []string{"persistence", "replication"},
})
```

## Advanced Usage

### Connection Context

Store connection-specific data using `Conn.SetContext()`:

```go
type ConnectionData struct {
    Authenticated bool
    Database      int
    ClientID      string
}

onOpened := func(c *redhub.Conn) (out []byte, action redhub.Action) {
    c.SetContext(&ConnectionData{
        Authenticated: false,
        Database:      0,
        ClientID:      generateID(),
    })
    return nil, redhub.None
}

onClosed := func(c *redhub.Conn, err error) (action redhub.Action) {
    ctx := c.Context().(*ConnectionData)
    // Cleanup connection data
    return redhub.None
}
```

### Command Pipelining

RedHub naturally supports command pipelining (sending multiple commands in a single network packet):

```bash
# Client sends multiple commands in one request
echo -e '*2\r\n$3\r\nSET\r\n$3\r\nkey1\r\n$5\r\nvalue1\r\n*2\r\n$3\r\nSET\r\n$3\r\nkey2\r\n$5\r\nvalue2\r\n*2\r\n$3\r\nGET\r\n$3\r\nkey1\r\n' | nc localhost 6379
```

### Multi-Protocol Support

RedHub supports three protocol types:

1. **RESP (Redis)** - Standard Redis protocol (commands starting with `*`)
2. **Tile38 Native** - Native Tile38 protocol (commands starting with `$`)
3. **Telnet** - Plain text commands

## Performance Benchmarks

### Test Environment

```
OS:     Debian Buster 10.6 64bit
CPU:    8 CPU cores
Memory: 64.0 GiB
Go:     go1.16.5 linux/amd64
```

### Benchmark Results

| Implementation | SET (req/sec) | GET (req/sec) |
|----------------|---------------|---------------|
| Redis 5.0.3 (single-threaded) | 2,306,060 | 3,096,742 |
| Redis 6.2.5 (single-threaded) | 2,076,325 | 2,652,801 |
| Redis 6.2.5 (multi-threaded) | 1,944,692 | 2,375,184 |
| RedCon (multi-threaded) | 2,332,742 | 14,654,162 |
| **RedHub (multi-threaded)** | **4,087,305** | **16,490,765** |

### Benchmark Command

```bash
redis-benchmark -h 127.0.0.1 -p 6379 -n 50000000 -t set,get -c 512 -P 1024 -q
```

<p align="center">
    <img 
        src="https://user-images.githubusercontent.com/12872991/134836128-423fd389-0fae-4e37-81c2-3b0066ed5f56.png" 
        border="0" alt="REDHUB Benchmarks">
    <br>
</p>

<p align="center">
    <img 
        src="https://user-images.githubusercontent.com/12872991/134836167-37c41c77-d77e-4ca8-96cb-4bab8ab65fa0.png" 
        border="0" alt="REDHUB Benchmarks">
    <br>
</p>

## Testing

### Run All Tests

```bash
go test ./...
```

### Run Tests with Coverage

```bash
go test -cover ./...
```

### Run Tests with Verbose Output

```bash
go test -v ./...
```

### Run Specific Package Tests

```bash
go test ./pkg/resp/...
```

### Run Specific Test

```bash
go test -run TestNewRedHub .
```

## Best Practices

### Performance Optimization

1. **Enable Multi-core**: Always enable `Multicore: true` in production
2. **Tune Buffer Sizes**: Adjust `ReadBufferCap`, `SocketRecvBuffer`, and `SocketSendBuffer` based on your workload
3. **Choose Load Balancing**: Use appropriate load balancing strategy (RoundRobin, LeastConnections, etc.)
4. **Avoid Blocking**: Never block in event handlers - use async operations
5. **Reuse Buffers**: Use buffer pools for temporary allocations

### Thread Safety

1. **Shared Data**: Always protect shared data with appropriate synchronization (mutexes)
2. **Connection Context**: Use `Conn.SetContext()` for per-connection data (thread-safe)
3. **Event Loop Handlers**: Handlers execute in event loop threads - avoid heavy computations

### Error Handling

1. **Protocol Errors**: Return proper RESP error messages using `resp.AppendError()`
2. **Connection Errors**: Log errors in `onClosed` handler
3. **Graceful Shutdown**: Handle server shutdown properly

## Contributing

We welcome contributions! Please follow these guidelines:

1. Fork the repository
2. Create a new branch from main/master
3. Make your changes with tests
4. Ensure all tests pass: `go test ./...`
5. Commit with DCO sign-off: `git commit -s -m "message"`
6. Push to your fork
7. Create a pull request

### Development Setup

```bash
# Clone the repository
git clone https://github.com/IceFireDB/redhub.git
cd redhub

# Install dependencies
go mod download

# Run tests
go test ./...

# Run the example
go run example/memory_kv/server.go
```

## License

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FIceFireDB%2fredhub.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2FIceFireDB%2fredhub?ref=badge_large)

This project is licensed under the MIT License - see the LICENSE file for details.

## Disclaimer

When you use this software, you agree and acknowledge that the author, maintainer, and contributor of this software are not responsible for any risks, costs, or problems you encounter. If you find a software defect or bug, please submit a patch to help improve it!

## Related Projects

- [IceFireDB](https://github.com/IceFireDB/IceFireDB) - A distributed database based on RedHub
- [gnet](https://github.com/panjf2000/gnet) - High-performance event-loop networking framework

## Documentation

- [GoDoc Reference](https://pkg.go.dev/github.com/IceFireDB/redhub)
- [Redis Protocol Specification](https://redis.io/docs/reference/protocol-spec/)
- [Effective Go](https://go.dev/doc/effective_go)

## Support

- GitHub Issues: [https://github.com/IceFireDB/redhub/issues](https://github.com/IceFireDB/redhub/issues)
- Discussions: [https://github.com/IceFireDB/redhub/discussions](https://github.com/IceFireDB/redhub/discussions)

## Acknowledgments

- Inspired by [redcon](https://github.com/tidwall/redcon)
- Built on top of [gnet](https://github.com/panjf2000/gnet)
- RESP protocol based on [Redis](https://redis.io)
