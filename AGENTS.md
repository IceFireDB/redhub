# AGENTS.md

This file provides comprehensive guidance for AI coding agents working on the RedHub codebase.

## Project Overview

RedHub is a high-performance RESP (Redis Serialization Protocol) server framework built in Go. It uses the RawEpoll model via the gnet library to achieve ultra-high throughput with multi-threaded support while maintaining low CPU resource consumption.

**Key Features:**
- Ultra high performance (exceeds Redis single-threaded and multi-threaded implementations)
- Fully multi-threaded support using event loops
- Low CPU resource consumption
- Full Redis protocol (RESP) compatibility
- Supports RESP, Tile38, and Telnet protocols
- Create Redis-compatible servers with minimal code

## Architecture

### Core Design Pattern

RedHub implements an event-driven architecture using the gnet framework:

1. **Event Loops**: Multiple event loops run in parallel (multi-threaded mode)
2. **Connection Pool**: Each connection has an associated buffer for command accumulation
3. **Command Pipeline**: Supports pipelining of multiple commands in a single read
4. **Handler Callbacks**: Three primary handlers for application logic:
   - `onOpened`: Called when a new connection is established
   - `onClosed`: Called when a connection is closed
   - `handler`: Called for each parsed command

### Threading Model

- **Single-core mode**: All connections handled by a single event loop
- **Multi-core mode**: Multiple event loops distribute connections using load balancing
- **Connection Buffering**: Each connection maintains its own buffer and command queue
- **Thread Safety**: Uses RWMutex for connection map synchronization (see redhub.go:53)

## Project Structure

```
redhub/
├── redhub.go                    # Main framework core (RedHub server implementation)
├── redhub_test.go               # Core framework tests
├── go.mod                       # Go module definition
├── go.sum                       # Dependency checksums
├── pkg/
│   └── resp/
│       ├── resp.go              # RESP protocol serialization/deserialization
│       ├── comparse.go          # Command parsing logic
│       ├── resp_test.go         # RESP protocol tests
│       └── comparse_test.go     # Command parsing tests
└── example/
    └── memory_kv/
        └── server.go            # Example Redis-compatible server (SET/GET/DEL/PING/QUIT)
```

## Core Components

### 1. RedHub Server (`redhub.go`)

**Key Types:**
- `RedHub`: Main server structure (line 48-54)
- `Options`: Server configuration options (line 30-45)
- `Conn`: Connection wrapper around gnet.Conn (line 25-27)
- `Action`: Post-event action type (line 13-22)

**Action Values:**
- `None`: No action
- `Close`: Close the connection
- `Shutdown`: Shutdown the server

**Server Options:**
```go
type Options struct {
    Multicore        bool              // Enable multi-core support
    LockOSThread     bool              // Lock OS thread
    ReadBufferCap    int               // Read buffer capacity
    LB               gnet.LoadBalancing // Load balancing strategy
    NumEventLoop     int               // Number of event loops
    ReusePort        bool              // Enable port reuse
    Ticker           bool              // Enable ticker
    TCPKeepAlive     time.Duration     // TCP keep-alive interval
    TCPKeepCount     int               // TCP keep-alive count
    TCPKeepInterval  time.Duration     // TCP keep-alive interval
    TCPNoDelay       gnet.TCPSocketOpt // TCP no-delay option
    SocketRecvBuffer int               // Socket receive buffer
    SocketSendBuffer int               // Socket send buffer
    EdgeTriggeredIO  bool              // Edge-triggered I/O
}
```

**Main Functions:**
- `NewRedHub(onOpened, onClosed, handler)`: Create new RedHub instance (line 63-75)
- `ListenAndServe(addr, options, rh)`: Start the server (line 161-205)

**Event Handlers:**
- `OnBoot(eng)`: Called when engine is ready (line 78-80)
- `OnShutdown(eng)`: Called when engine is shutting down (line 83-84)
- `OnOpen(c)`: Called when new connection opens (line 87-93)
- `OnClose(c, err)`: Called when connection closes (line 96-101)
- `OnTraffic(c)`: Called when data is received (line 104-153)
- `OnTick()`: Called on timer ticks (line 156-158)

### 2. RESP Protocol Package (`pkg/resp/`)

#### resp.go - Protocol Serialization

**RESP Types:**
```go
const (
    Integer = ':'  // Integers (e.g., :1000\r\n)
    String  = '+'  // Simple strings (e.g., +OK\r\n)
    Bulk    = '$'  // Bulk strings (e.g., $6\r\nfoobar\r\n)
    Array   = '*'  // Arrays (e.g., *2\r\n$3\r\nGET\r\n$3\r\nkey\r\n)
    Error   = '-'  // Errors (e.g., -ERR unknown command\r\n)
)
```

**Key Functions:**

**Reading/Parsing:**
- `ReadNextRESP(b []byte) (n int, resp RESP)`: Parse next RESP value (line 45-134)
- `ReadNextCommand(packet, argsbuf)`: Parse next command (line 159-226)
- `ForEach(iter func(resp RESP) bool)`: Iterate over array elements (line 32-41)

**Writing/Serializing:**
- `AppendInt(b []byte, n int64)`: Append integer (line 378-380)
- `AppendString(b []byte, s string)`: Append simple string (line 402-406)
- `AppendBulk(b []byte, bulk []byte)`: Append bulk bytes (line 388-392)
- `AppendBulkString(b []byte, bulk string)`: Append bulk string (line 395-399)
- `AppendArray(b []byte, n int)`: Append array header (line 383-385)
- `AppendError(b []byte, s string)`: Append error (line 409-413)
- `AppendNull(b []byte)`: Append null value (line 440-442)
- `AppendOK(b []byte)`: Append OK response (line 416-418)
- `AppendAny(b []byte, v interface{})`: Append any Go type (line 503-598)

**AppendAny Type Mapping:**
- `nil` → null
- `error` → error (adds "ERR " prefix if needed)
- `string` → bulk string
- `[]byte` → bulk bytes
- `bool` → bulk string ("0" or "1")
- Numbers (int, int64, uint64, float64) → bulk string
- `[]Type` → array
- `map[K]V` → array with key/value pairs
- `SimpleString` → simple string
- `SimpleInt` → integer
- `Marshaler` → custom RESP

#### comparse.go - Command Parsing

**Key Types:**
```go
type Command struct {
    Raw  []byte   // Raw RESP message
    Args [][]byte // Command arguments
}
```

**Key Functions:**
- `ReadCommands(buf []byte) ([]Command, []byte, error)`: Parse multiple commands from buffer (line 40-78)
- `parseRESPCommand(b []byte)`: Parse RESP format command (line 81-134)
- `parsePlainTextCommand(b []byte)`: Parse plain text command (line 137-155)

**Protocol Support:**
- Standard RESP (Redis protocol): Commands starting with `*`
- Tile38 native protocol: Commands starting with `$`
- Telnet protocol: Plain text commands

**Command Parsing Flow:**
1. Check first byte to determine protocol type
2. Parse according to protocol specification
3. Return parsed command or incomplete data (if more bytes needed)
4. Return error for malformed commands

## Quick Start

### Installation
```bash
go get -u github.com/IceFireDB/redhub
```

### Running the Example
```bash
go run example/memory_kv/server.go
```

### Basic Server Implementation

```go
package main

import (
    "github.com/IceFireDB/redhub"
    "github.com/IceFireDB/redhub/pkg/resp"
)

func main() {
    // Create RedHub instance with handlers
    rh := redhub.NewRedHub(
        // OnOpen: Connection opened
        func(c *redhub.Conn) (out []byte, action redhub.Action) {
            return nil, redhub.None
        },
        // OnClose: Connection closed
        func(c *redhub.Conn, err error) (action redhub.Action) {
            return redhub.None
        },
        // Handler: Process commands
        func(cmd resp.Command, out []byte) ([]byte, redhub.Action) {
            // Process command and return response
            return out, redhub.None
        },
    )
    
    // Start server
    err := redhub.ListenAndServe("tcp://127.0.0.1:6379", redhub.Options{
        Multicore: true,
    }, rh)
    if err != nil {
        panic(err)
    }
}
```

## Development Guide

### Go Requirements
- **Go version**: 1.24.0 (see go.mod:3)

### Dependencies
- `github.com/panjf2000/gnet/v2 v2.9.7`: High-performance event-loop networking framework
- `github.com/stretchr/testify v1.11.1`: Testing framework with assertions
- Indirect dependencies:
  - `github.com/valyala/bytebufferpool`: Byte buffer pooling
  - `go.uber.org/zap`: Logging
  - `go.uber.org/multierr`: Multi-error handling
  - `golang.org/x/sync`: Sync utilities

### Building
```bash
go build
```

### Testing

**Run all tests:**
```bash
go test ./...
```

**Run with coverage:**
```bash
go test -cover ./...
```

**Run with verbose output:**
```bash
go test -v ./...
```

**Run specific package:**
```bash
go test ./pkg/resp/...
```

**Run specific test:**
```bash
go test -run TestNewRedHub .
```

**Test files:**
- `redhub_test.go`: Core framework tests (313 lines)
- `pkg/resp/resp_test.go`: RESP protocol tests
- `pkg/resp/comparse_test.go`: Command parsing tests

### Code Style
- Follow standard Go conventions (gofmt)
- Use standard Go idioms
- Add tests for all code changes
- Ensure all tests pass before committing

## Implementation Guidelines

### Writing Command Handlers

Command handlers receive a `resp.Command` struct:
```go
type Command struct {
    Raw  []byte   // Raw RESP bytes
    Args [][]byte // Parsed arguments
}
```

Example handler:
```go
func(cmd resp.Command, out []byte) ([]byte, redhub.Action) {
    // Command name is first argument (case-insensitive)
    cmdName := strings.ToLower(string(cmd.Args[0]))
    
    switch cmdName {
    case "set":
        // Validate arguments
        if len(cmd.Args) != 3 {
            return resp.AppendError(out, "ERR wrong number of arguments"), redhub.None
        }
        key := cmd.Args[1]
        value := cmd.Args[2]
        // Store value...
        return resp.AppendString(out, "OK"), redhub.None
        
    case "get":
        if len(cmd.Args) != 2 {
            return resp.AppendError(out, "ERR wrong number of arguments"), redhub.None
        }
        key := cmd.Args[1]
        // Retrieve value...
        return resp.AppendBulk(out, value), redhub.None
        
    case "quit":
        return resp.AppendString(out, "OK"), redhub.Close
    }
    
    return resp.AppendError(out, "ERR unknown command '"+string(cmd.Args[0])+"'"), redhub.None
}
```

### Connection Management

Use the Conn context to store connection-specific data:
```go
onOpened := func(c *redhub.Conn) (out []byte, action redhub.Action) {
    c.SetContext(&ConnectionData{
        Authenticated: false,
        ClientID: generateID(),
    })
    return nil, redhub.None
}

onClosed := func(c *redhub.Conn, err error) (action redhub.Action) {
    ctx := c.Context().(*ConnectionData)
    // Cleanup connection data...
    return redhub.None
}
```

### Multi-threading Considerations

1. **Locking**: Use appropriate synchronization for shared data
2. **Connection Buffers**: Each connection has its own buffer (thread-safe)
3. **Event Loops**: Handlers execute in event loop threads
4. **Avoid Blocking**: Never block in handlers - use async operations
5. **Thread-local Data**: Use `Conn.SetContext()` for per-connection data

## RESP Protocol Details

### RESP Message Formats

**Simple String:**
```
+OK\r\n
```

**Error:**
```
-ERR unknown command\r\n
```

**Integer:**
```
:1000\r\n
```

**Bulk String:**
```
$6\r\nfoobar\r\n
```

**Null Bulk String:**
```
$-1\r\n
```

**Array:**
```
*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n
```

**Null Array:**
```
*-1\r\n
```

### Protocol Compliance

- Redis protocol version 2 (RESP2)
- Supports pipelining (multiple commands in one network packet)
- Supports multi-bulk commands
- Client compatibility with standard Redis clients

## Performance Guidelines

### Performance Characteristics

Based on benchmark results (Debian Buster, 8 CPU cores, 64GB RAM):

- **RedHub SET**: ~4,087,305 req/sec (vs Redis: ~2,300,000)
- **RedHub GET**: ~16,490,765 req/sec (vs Redis: ~3,000,000)

Performance exceeds Redis single-threaded and multi-threaded implementations.

### Performance Optimization Tips

1. **Avoid Memory Allocations**: Reuse buffers when possible
2. **Use Multi-core**: Enable `Options.Multicore` for production
3. **Tune Buffers**: Adjust `ReadBufferCap`, `SocketRecvBuffer`, `SocketSendBuffer`
4. **Load Balancing**: Choose appropriate `LB` strategy
5. **Event Loops**: Set `NumEventLoop` based on CPU cores
6. **Profile**: Use pprof to identify bottlenecks

### Profiling

Enable pprof in example:
```go
import _ "net/http/pprof"

go func() {
    http.ListenAndServe(":8888", nil)
}()
```

Benchmark:
```bash
redis-benchmark -h 127.0.0.1 -p 6379 -n 10000000 -t set,get -c 512 -P 1024 -q
```

## Contribution Workflow

1. Create an issue to discuss your change
2. Fork the repository
3. Create a new branch from main/master
4. Make your changes with tests
5. Ensure all tests pass: `go test ./...`
6. Commit with DCO sign-off
7. Push to your fork
8. Create a pull request

## Commit Guidelines

- Every commit must be signed with DCO (Developer Certificate of Origin)
- Sign automatically: `git commit -s -m "message"`
- Or add manually: `Signed-off-by: Your Name <your.email@example.com>"`
- If you forgot to sign: `git commit --amend --no-edit --signoff` then `git push --force-with-lease`
- Write clear, descriptive commit messages following conventional commits

## Pull Request Guidelines

- Reference the related issue in your PR description
- All code changes must include tests
- Wait for CI checks to complete and pass
- Maintainers review and merge within a few days
- Be responsive to review comments

## Common Issues

### Import Errors
- Ensure Go 1.24.0 is installed: `go version`
- Run `go mod tidy` to resolve dependencies

### Test Failures
- Check Go version compatibility
- Run `go mod tidy`
- Verify test environment

### Performance Issues
- Profile with pprof before and after changes
- Compare with baseline benchmarks
- Consider memory allocation patterns

### Connection Issues
- Check TCP socket options
- Verify load balancing configuration
- Review event loop settings

### Memory Leaks
- Ensure proper cleanup in `onClosed` handler
- Check for context data cleanup
- Profile with pprof

## Testing Best Practices

### Unit Tests
- Test each function independently
- Use table-driven tests for multiple cases
- Mock external dependencies
- Test edge cases and error conditions

### Integration Tests
- Test full request/response cycle
- Test pipelining scenarios
- Test connection lifecycle
- Use real Redis clients for compatibility testing

### Test Coverage
- Aim for high test coverage
- Test all code paths
- Test error handling
- Test concurrent scenarios

## Additional Resources

- [Go Documentation](https://golang.org/doc/)
- [gnet Framework](https://github.com/panjf2000/gnet)
- [Redis Protocol Specification](https://redis.io/docs/reference/protocol-spec/)
- [Go Modules Reference](https://go.dev/ref/mod)
- [Effective Go](https://go.dev/doc/effective_go)

## Notes for AI Agents

### Key Files to Understand First
1. `redhub.go` - Core server implementation
2. `pkg/resp/resp.go` - RESP protocol handling
3. `example/memory_kv/server.go` - Complete working example

### Common Patterns
- All handlers return `(response []byte, action Action)`
- Use `resp.Append*` functions to build responses
- Use `Conn.SetContext()` for per-connection data
- Handlers should be non-blocking

### When Modifying Code
- Maintain RESP protocol compatibility
- Consider performance impact
- Add/update tests
- Update documentation if needed

### Testing Changes
- Run `go test ./...` before committing
- Test with Redis clients
- Benchmark if performance-critical
- Check for memory leaks

## Protocol Support Summary

**Supported Protocols:**
1. **RESP (Redis)**: Full support, primary protocol
2. **Tile38 Native**: Partial support for native Tile38 commands
3. **Telnet**: Basic plain-text command support

**Redis Commands (in example):**
- `SET key value` - Set key-value pair
- `GET key` - Get value by key
- `DEL key` - Delete key
- `PING` - Ping server (responds PONG)
- `QUIT` - Close connection

**Extend with your own commands by implementing handlers.**
