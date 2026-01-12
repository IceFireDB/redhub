// Package redhub provides a high-performance RESP (Redis Serialization Protocol) server framework.
// It is built on top of the gnet library and uses the RawEpoll model to achieve ultra-high throughput
// with multi-threaded support while maintaining low CPU resource consumption.
//
// RedHub is designed to help developers create Redis-compatible servers with minimal code.
// It supports the full RESP2 protocol and is compatible with standard Redis clients.
//
// # Basic Usage
//
// To create a simple Redis-compatible server:
//
//	rh := redhub.NewRedHub(
//	    func(c *redhub.Conn) (out []byte, action redhub.Action) {
//	        // Called when a new connection is established
//	        return nil, redhub.None
//	    },
//	    func(c *redhub.Conn, err error) (action redhub.Action) {
//	        // Called when a connection is closed
//	        return redhub.None
//	    },
//	    func(cmd resp.Command, out []byte) ([]byte, redhub.Action) {
//	        // Called for each parsed command
//	        cmdName := strings.ToLower(string(cmd.Args[0]))
//	        switch cmdName {
//	        case "ping":
//	            return resp.AppendString(out, "PONG"), redhub.None
//	        default:
//	            return resp.AppendError(out, "ERR unknown command"), redhub.None
//	        }
//	    },
//	)
//
//	err := redhub.ListenAndServe("tcp://127.0.0.1:6379", redhub.Options{
//	    Multicore: true,
//	}, rh)
//
// # Architecture
//
// RedHub implements an event-driven architecture using multiple event loops that run in parallel
// (in multi-core mode). Each connection has an associated buffer for command accumulation,
// and commands are parsed using the RESP protocol parser from the resp package.
//
// # Threading Model
//
// - Single-core mode: All connections are handled by a single event loop
// - Multi-core mode: Multiple event loops distribute connections using load balancing strategies
// - Connection Buffering: Each connection maintains its own buffer and command queue
// - Thread Safety: Uses RWMutex for connection map synchronization
//
// # Performance
//
// RedHub is optimized for high performance and can handle millions of requests per second
// depending on the hardware and configuration. See the benchmarks in the project README
// for detailed performance comparisons with Redis and other implementations.
package redhub

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IceFireDB/redhub/pkg/resp"
	"github.com/panjf2000/gnet/v2"
)

// Action represents the type of action to be taken after an event handler completes.
// Event handlers (OnOpen, OnClose, Handler) return an Action value to control
// the server's behavior after processing the event.
type Action int

const (
	// None indicates that no action should be taken following an event.
	// The connection remains open and the server continues processing.
	None Action = iota

	// Close indicates that the connection should be closed.
	// This is typically returned when processing a QUIT command or when
	// an error condition requires closing the connection.
	Close

	// Shutdown indicates that the entire server should be shut down.
	// This is rarely used in normal operation but can be used to implement
	// graceful shutdown functionality.
	Shutdown
)

// Conn wraps a gnet.Conn and provides additional functionality for connection management.
// It is passed to the OnOpen and OnClose handlers to allow application code to
// store connection-specific data and perform connection-level operations.
type Conn struct {
	gnet.Conn
}

// SetContext sets the connection-specific context data.
// This can be used to store application-specific data such as authentication state,
// selected database, or any other per-connection information.
//
// The context is accessible via the Context() method and is automatically
// cleaned up when the connection is closed.
func (c *Conn) SetContext(ctx interface{}) {
	c.Conn.SetContext(ctx)
}

// Context returns the connection-specific context data.
// Returns the data that was previously set using SetContext.
// Returns nil if no context has been set.
func (c *Conn) Context() interface{} {
	return c.Conn.Context()
}

// Options defines the configuration options for the RedHub server.
// These options control various aspects of server behavior including threading,
// buffer sizes, network settings, and performance tuning.
//
// Most options have sensible defaults and only need to be changed for specific use cases.
type Options struct {
	// Multicore enables multi-core support. When true, multiple event loops are created
	// and connections are distributed across them using the configured load balancing strategy.
	// This is recommended for production environments with high connection counts.
	// Default: false
	Multicore bool

	// LockOSThread locks the OS thread for each event loop. This can improve performance
	// in certain scenarios but may reduce the overall number of connections that can be handled.
	// Default: false
	LockOSThread bool

	// ReadBufferCap sets the capacity of the read buffer in bytes. Larger buffers can
	// improve throughput for workloads with large requests or responses but use more memory.
	// Default: 64KB
	ReadBufferCap int

	// LB specifies the load balancing strategy used to distribute connections across
	// event loops when Multicore is enabled. Available strategies include:
	//   - RoundRobin: Distribute connections evenly across loops
	//   - LeastConnections: Assign to loop with fewest active connections
	//   - SourceAddrHash: Hash based on client address
	// Default: gnet.RoundRobin
	LB gnet.LoadBalancing

	// NumEventLoop specifies the number of event loops to create. If 0, the number
	// of CPU cores is used. This option is only effective when Multicore is true.
	// Default: 0 (runtime.NumCPU())
	NumEventLoop int

	// ReusePort enables the SO_REUSEPORT socket option, allowing multiple sockets
	// to bind to the same address and port. This can improve connection acceptance
	// performance but is only available on certain operating systems.
	// Default: false
	ReusePort bool

	// Ticker enables periodic ticker events. When true, the OnTick handler is called
	// at regular intervals. Useful for implementing periodic tasks such as cleanup,
	// stats collection, or timeout handling.
	// Default: false
	Ticker bool

	// TCPKeepAlive sets the TCP keep-alive interval. If non-zero, TCP keep-alive
	// probes are sent at the specified interval to detect dead connections.
	// Default: 0 (disabled)
	TCPKeepAlive time.Duration

	// TCPKeepCount sets the number of unacknowledged keep-alive probes before
	// considering the connection dead. Only effective if TCPKeepAlive is set.
	// Default: 0 (system default)
	TCPKeepCount int

	// TCPKeepInterval sets the interval between keep-alive probes when they are
	// not acknowledged. Only effective if TCPKeepAlive is set.
	// Default: 0 (system default)
	TCPKeepInterval time.Duration

	// TCPNoDelay sets the TCP_NODELAY socket option. When true, disables Nagle's
	// algorithm, sending data immediately rather than buffering it. This reduces
	// latency but may increase network overhead.
	// Default: gnet.TCPSocketOpt(1) (enabled)
	TCPNoDelay gnet.TCPSocketOpt

	// SocketRecvBuffer sets the size of the socket receive buffer in bytes.
	// Larger buffers can handle bursts of data but use more memory.
	// Default: 0 (system default)
	SocketRecvBuffer int

	// SocketSendBuffer sets the size of the socket send buffer in bytes.
	// Larger buffers can handle bursty sends but use more memory.
	// Default: 0 (system default)
	SocketSendBuffer int

	// EdgeTriggeredIO enables edge-triggered I/O mode when available.
	// This can reduce the number of system calls but requires careful handling.
	// Default: false
	EdgeTriggeredIO bool

	// TLSListenEnable enables TLS support. When true, a TLS listener is started
	// alongside the TCP listener. TLS connections are proxied to the TCP server.
	// Default: false
	TLSListenEnable bool

	// TLSCertFile specifies the path to the TLS certificate file.
	// Required when TLSListenEnable is true.
	// Default: ""
	TLSCertFile string

	// TLSKeyFile specifies the path to the TLS private key file.
	// Required when TLSListenEnable is true.
	// Default: ""
	TLSKeyFile string

	// TLSAddr specifies the address for the TLS listener.
	// If empty, it's derived from the main TCP address by changing the port
	// (e.g., tcp://127.0.0.1:6379 -> tcp://127.0.0.1:6380).
	// Default: ""
	TLSAddr string
}

// RedHub represents the main server structure that manages connections and command processing.
// It implements the gnet.EventHandler interface and is typically created using NewRedHub.
//
// RedHub maintains a map of connections to their associated buffers, allowing each
// connection to accumulate data across multiple reads until complete commands are parsed.
type RedHub struct {
	onOpened     func(c *Conn) (out []byte, action Action)
	onClosed     func(c *Conn, err error) (action Action)
	handler      func(cmd resp.Command, out []byte) ([]byte, Action)
	redHubBufMap map[gnet.Conn]*connBuffer
	connSync     *sync.RWMutex
	mu           sync.Mutex
	addr         string
	tcpAddr      string
	running      bool
	engine       gnet.Engine
	tlsListener  net.Listener
}

// connBuffer holds the buffer and commands for each connection.
// This structure is maintained internally by RedHub and is not exposed to users.
//
// The buffer accumulates incoming data until complete commands can be parsed.
// Once commands are parsed, they are stored in the command slice for processing.
type connBuffer struct {
	buf     bytes.Buffer   // Accumulates incoming data from the network
	command []resp.Command // Stores parsed commands waiting to be processed
}

// NewRedHub creates a new RedHub instance with the specified event handlers.
//
// The handlers allow application code to respond to connection lifecycle events
// and process incoming commands.
//
// Parameters:
//   - onOpened: Called when a new connection is established. The connection
//     object is provided, allowing initialization of connection-specific data.
//     Returns any initial response data and an action (typically None).
//   - onClosed: Called when a connection is closed. The connection object and
//     any error that caused the close are provided. Returns an action.
//   - handler: Called for each parsed command from the connection. The command
//     contains the raw RESP bytes and parsed arguments. The response buffer
//     is provided for building the response. Returns the response data and
//     an action (None, Close, or Shutdown).
//
// The returned RedHub instance can then be passed to ListenAndServe to start
// the server.
func NewRedHub(
	onOpened func(c *Conn) (out []byte, action Action),
	onClosed func(c *Conn, err error) (action Action),
	handler func(cmd resp.Command, out []byte) ([]byte, Action),
) *RedHub {
	return &RedHub{
		redHubBufMap: make(map[gnet.Conn]*connBuffer),
		connSync:     &sync.RWMutex{},
		onOpened:     onOpened,
		onClosed:     onClosed,
		handler:      handler,
	}
}

// OnBoot is called by gnet when the server is ready to accept connections.
// This is part of the gnet.EventHandler interface.
//
// The engine parameter provides access to server-wide operations.
// Typically returns gnet.None to indicate normal startup.
func (rs *RedHub) OnBoot(eng gnet.Engine) (action gnet.Action) {
	rs.mu.Lock()
	rs.engine = eng
	rs.mu.Unlock()
	return gnet.None
}

// OnShutdown is called by gnet when the server is shutting down.
// This is part of the gnet.EventHandler interface.
//
// The engine parameter provides access to server-wide operations during shutdown.
// This can be used to perform cleanup tasks or notify application code.
func (rs *RedHub) OnShutdown(eng gnet.Engine) {
}

// OnOpen is called by gnet when a new connection is opened.
// This is part of the gnet.EventHandler interface.
//
// A new buffer is created for the connection to accumulate incoming data,
// and then the application's onOpened handler is called.
func (rs *RedHub) OnOpen(c gnet.Conn) (out []byte, action gnet.Action) {
	rs.connSync.Lock()
	rs.redHubBufMap[c] = new(connBuffer)
	rs.connSync.Unlock()
	out, act := rs.onOpened(&Conn{Conn: c})
	return out, gnet.Action(act)
}

// OnClose is called by gnet when a connection is closed.
// This is part of the gnet.EventHandler interface.
//
// The connection's buffer is removed from the map to free memory,
// and then the application's onClosed handler is called.
func (rs *RedHub) OnClose(c gnet.Conn, err error) (action gnet.Action) {
	rs.connSync.Lock()
	delete(rs.redHubBufMap, c)
	rs.connSync.Unlock()
	return gnet.Action(rs.onClosed(&Conn{Conn: c}, err))
}

// OnTraffic is called by gnet when data is received from a connection.
// This is part of the gnet.EventHandler interface and is the core
// of the request processing pipeline.
//
// The function:
// 1. Reads all available data from the connection
// 2. Appends it to the connection's buffer
// 3. Parses complete commands from the buffer
// 4. Processes each command through the handler
// 5. Sends responses back to the client
// 6. Handles incomplete commands by keeping remaining data in the buffer
func (rs *RedHub) OnTraffic(c gnet.Conn) (action gnet.Action) {
	rs.connSync.RLock()
	cb, ok := rs.redHubBufMap[c]
	rs.connSync.RUnlock()

	if !ok {
		_, _ = c.Write(resp.AppendError(nil, "ERR Client is closed"))
		return gnet.None
	}

	buf, _ := c.Next(-1)
	if len(buf) == 0 {
		return gnet.None
	}

	cb.buf.Write(buf)
	cmds, lastbyte, err := resp.ReadCommands(cb.buf.Bytes())
	if err != nil {
		_, _ = c.Write(resp.AppendError(nil, "ERR "+err.Error()))
		return gnet.None
	}

	cb.command = append(cb.command, cmds...)
	cb.buf.Reset()

	if len(lastbyte) == 0 {
		var out []byte
		for len(cb.command) > 0 {
			cmd := cb.command[0]
			cb.command = cb.command[1:]

			var status Action
			out, status = rs.handler(cmd, out)

			if status == Close {
				if len(out) > 0 {
					_, _ = c.Write(out)
				}
				return gnet.Close
			}
		}
		if len(out) > 0 {
			_, _ = c.Write(out)
		}
	} else {
		cb.buf.Write(lastbyte)
	}

	return gnet.None
}

// OnTick is called by gnet on a periodic timer when Ticker is enabled.
// This is part of the gnet.EventHandler interface.
//
// Returns the delay until the next tick and an action.
// Typically returns (0, gnet.None) to disable further ticks.
func (rs *RedHub) OnTick() (delay time.Duration, action gnet.Action) {
	return 0, gnet.None
}

// deriveTLSAddr derives a TLS address from the TCP address by incrementing the port.
func deriveTLSAddr(tcpAddr string) string {
	if !strings.HasPrefix(tcpAddr, "tcp://") {
		return ""
	}

	hostPort := strings.TrimPrefix(tcpAddr, "tcp://")
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return ""
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return ""
	}

	return "tcp://" + net.JoinHostPort(host, strconv.Itoa(port+1))
}

// startTLSListener starts the TLS listener that proxies connections to the TCP server.
func (rs *RedHub) startTLSListener(options Options) error {
	cert, err := tls.LoadX509KeyPair(options.TLSCertFile, options.TLSKeyFile)
	if err != nil {
		return err
	}

	tlsAddr := options.TLSAddr
	if tlsAddr == "" {
		tlsAddr = deriveTLSAddr(rs.tcpAddr)
		if tlsAddr == "" {
			return errors.New("failed to derive TLS address from TCP address")
		}
	}

	listenAddr := tlsAddr
	if strings.HasPrefix(tlsAddr, "tcp://") {
		listenAddr = strings.TrimPrefix(tlsAddr, "tcp://")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	rs.tlsListener, err = tls.Listen("tcp", listenAddr, tlsConfig)
	if err != nil {
		return err
	}

	tcpForwardAddr := rs.tcpAddr
	if strings.HasPrefix(tcpForwardAddr, "tcp://") {
		tcpForwardAddr = strings.TrimPrefix(tcpForwardAddr, "tcp://")
	}

	go rs.acceptTLSConnections(tcpForwardAddr)

	return nil
}

// acceptTLSConnections accepts TLS connections and forwards them to the TCP server.
func (rs *RedHub) acceptTLSConnections(tcpAddr string) {
	for {
		tlsConn, err := rs.tlsListener.Accept()
		if err != nil {
			if !rs.running {
				return
			}
			continue
		}

		go rs.handleTLSConn(tlsConn, tcpAddr)
	}
}

// handleTLSConn handles a single TLS connection by forwarding data to the TCP server.
func (rs *RedHub) handleTLSConn(tlsConn net.Conn, tcpAddr string) {
	defer tlsConn.Close()

	tcpConn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		return
	}
	defer tcpConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := tlsConn.Read(buf)
			if err != nil {
				return
			}
			_, err = tcpConn.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := tcpConn.Read(buf)
			if err != nil {
				return
			}
			_, err = tlsConn.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()

	wg.Wait()
}

// ListenAndServe starts the RedHub server on the specified address with the given options.
//
// This is the main entry point for starting a RedHub server. The address should be
// in the format "tcp://host:port" (e.g., "tcp://127.0.0.1:6379").
//
// The function blocks until the server is stopped, either by a Shutdown action or
// by an error.
//
// Parameters:
//   - addr: The address to listen on in format "scheme://host:port"
//   - options: Server configuration options
//   - rh: The RedHub instance created by NewRedHub
//
// Returns an error if the server fails to start. Otherwise, blocks until shutdown.
//
// Example:
//
//	err := redhub.ListenAndServe("tcp://127.0.0.1:6379", redhub.Options{
//	    Multicore: true,
//	    NumEventLoop: 8,
//	}, rh)
//	if err != nil {
//	    log.Fatal(err)
//	}
func ListenAndServe(addr string, options Options, rh *RedHub) error {
	if options.TLSListenEnable {
		if options.TLSCertFile == "" || options.TLSKeyFile == "" {
			return errors.New("TLSListenEnable requires TLSCertFile and TLSKeyFile")
		}
	}

	var opts []gnet.Option

	if options.Multicore {
		opts = append(opts, gnet.WithMulticore(true))
	}
	if options.LockOSThread {
		opts = append(opts, gnet.WithLockOSThread(true))
	}
	if options.ReadBufferCap > 0 {
		opts = append(opts, gnet.WithReadBufferCap(options.ReadBufferCap))
	}
	if options.NumEventLoop > 0 {
		opts = append(opts, gnet.WithNumEventLoop(options.NumEventLoop))
	} else if options.LB != gnet.RoundRobin {
		opts = append(opts, gnet.WithLoadBalancing(options.LB))
	}
	if options.ReusePort {
		opts = append(opts, gnet.WithReusePort(true))
	}
	if options.Ticker {
		opts = append(opts, gnet.WithTicker(true))
	}
	if options.TCPKeepAlive > 0 {
		opts = append(opts, gnet.WithTCPKeepAlive(options.TCPKeepAlive))
	}
	if options.TCPKeepCount > 0 {
		opts = append(opts, gnet.WithTCPKeepCount(options.TCPKeepCount))
	}
	if options.TCPKeepInterval > 0 {
		opts = append(opts, gnet.WithTCPKeepInterval(options.TCPKeepInterval))
	}
	opts = append(opts, gnet.WithTCPNoDelay(options.TCPNoDelay))
	if options.SocketRecvBuffer > 0 {
		opts = append(opts, gnet.WithSocketRecvBuffer(options.SocketRecvBuffer))
	}
	if options.SocketSendBuffer > 0 {
		opts = append(opts, gnet.WithSocketSendBuffer(options.SocketSendBuffer))
	}
	if options.EdgeTriggeredIO {
		opts = append(opts, gnet.WithEdgeTriggeredIO(true))
	}

	rh.mu.Lock()
	rh.addr = addr
	rh.tcpAddr = addr
	rh.running = true
	rh.mu.Unlock()

	if options.TLSListenEnable {
		if err := rh.startTLSListener(options); err != nil {
			rh.mu.Lock()
			rh.running = false
			rh.mu.Unlock()
			return err
		}
	}

	err := gnet.Run(rh, addr, opts...)

	rh.mu.Lock()
	rh.running = false
	rh.mu.Unlock()

	if rh.tlsListener != nil {
		rh.tlsListener.Close()
	}

	return err
}

// Close gracefully shuts down the RedHub server.
//
// This method stops the server and closes all active connections. It is safe to call
// multiple times. If the server is not currently running, it returns an error.
//
// Returns an error if the server is not running or if the shutdown fails.
func (rs *RedHub) Close() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.running {
		return errors.New("server not running")
	}

	rs.running = false

	if rs.tlsListener != nil {
		_ = rs.tlsListener.Close()
	}

	return rs.engine.Stop(context.Background())
}
