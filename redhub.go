package redhub

import (
	"bytes"
	"sync"
	"time"

	"github.com/IceFireDB/redhub/pkg/resp"
	"github.com/panjf2000/gnet/v2"
)

// Action represents the type of action to be taken after an event
type Action int

const (
	// None indicates that no action should occur following an event
	None Action = iota
	// Close indicates that the connection should be closed
	Close
	// Shutdown indicates that the server should be shut down
	Shutdown
)

// Conn wraps a gnet.Conn
type Conn struct {
	gnet.Conn
}

// Options defines the configuration options for the RedHub server
type Options struct {
	Multicore        bool
	LockOSThread     bool
	ReadBufferCap    int
	LB               gnet.LoadBalancing
	NumEventLoop     int
	ReusePort        bool
	Ticker           bool
	TCPKeepAlive     time.Duration
	TCPKeepCount     int
	TCPKeepInterval  time.Duration
	TCPNoDelay       gnet.TCPSocketOpt
	SocketRecvBuffer int
	SocketSendBuffer int
	EdgeTriggeredIO  bool
}

// RedHub represents the main server structure
type RedHub struct {
	onOpened     func(c *Conn) (out []byte, action Action)
	onClosed     func(c *Conn, err error) (action Action)
	handler      func(cmd resp.Command, out []byte) ([]byte, Action)
	redHubBufMap map[gnet.Conn]*connBuffer
	connSync     *sync.RWMutex
}

// connBuffer holds the buffer and commands for each connection
type connBuffer struct {
	buf     bytes.Buffer
	command []resp.Command
}

// NewRedHub creates a new RedHub instance
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

// OnBoot fires when the engine is ready for accepting connections
func (rs *RedHub) OnBoot(eng gnet.Engine) (action gnet.Action) {
	return gnet.None
}

// OnShutdown fires when the engine is being shut down
func (rs *RedHub) OnShutdown(eng gnet.Engine) {
}

// OnOpen fires when a new connection is opened
func (rs *RedHub) OnOpen(c gnet.Conn) (out []byte, action gnet.Action) {
	rs.connSync.Lock()
	rs.redHubBufMap[c] = new(connBuffer)
	rs.connSync.Unlock()
	out, act := rs.onOpened(&Conn{Conn: c})
	return out, gnet.Action(act)
}

// OnClose fires when a connection is closed
func (rs *RedHub) OnClose(c gnet.Conn, err error) (action gnet.Action) {
	rs.connSync.Lock()
	delete(rs.redHubBufMap, c)
	rs.connSync.Unlock()
	return gnet.Action(rs.onClosed(&Conn{Conn: c}, err))
}

// OnTraffic fires when a socket receives data from the remote
func (rs *RedHub) OnTraffic(c gnet.Conn) (action gnet.Action) {
	var out []byte
	rs.connSync.RLock()
	cb, ok := rs.redHubBufMap[c]
	rs.connSync.RUnlock()

	if !ok {
		c.AsyncWrite(resp.AppendError(nil, "ERR Client is closed"), nil)
		return gnet.None
	}

	buf, _ := c.Next(-1)
	if len(buf) == 0 {
		return gnet.None
	}

	cb.buf.Write(buf)
	cmds, lastbyte, err := resp.ReadCommands(cb.buf.Bytes())
	if err != nil {
		c.AsyncWrite(resp.AppendError(nil, "ERR "+err.Error()), nil)
		return gnet.None
	}

	cb.command = append(cb.command, cmds...)
	cb.buf.Reset()

	if len(lastbyte) == 0 {
		for len(cb.command) > 0 {
			cmd := cb.command[0]
			cb.command = cb.command[1:]

			var status Action
			result, status := rs.handler(cmd, out)
			if len(result) > 0 {
				c.AsyncWrite(result, nil)
			}

			if status == Close {
				return gnet.Close
			}
		}
	} else {
		cb.buf.Write(lastbyte)
	}

	return gnet.None
}

// OnTick fires immediately after the engine starts
func (rs *RedHub) OnTick() (delay time.Duration, action gnet.Action) {
	return 0, gnet.None
}

// ListenAndServe starts the RedHub server
func ListenAndServe(addr string, options Options, rh *RedHub) error {
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

	return gnet.Run(rh, addr, opts...)
}
