package redhub

import (
	"bytes"
	"sync"
	"time"

	"github.com/IceFireDB/redhub/pkg/resp"
	"github.com/panjf2000/gnet"
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
	TCPNoDelay       gnet.TCPSocketOpt
	SocketRecvBuffer int
	SocketSendBuffer int
	Codec            gnet.ICodec
}

// RedHub represents the main server structure
type RedHub struct {
	*gnet.EventServer
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

// OnOpened is called when a new connection is opened
func (rs *RedHub) OnOpened(c gnet.Conn) (out []byte, action gnet.Action) {
	rs.connSync.Lock()
	rs.redHubBufMap[c] = new(connBuffer)
	rs.connSync.Unlock()
	out, act := rs.onOpened(&Conn{Conn: c})
	return out, gnet.Action(act)
}

// OnClosed is called when a connection is closed
func (rs *RedHub) OnClosed(c gnet.Conn, err error) (action gnet.Action) {
	rs.connSync.Lock()
	delete(rs.redHubBufMap, c)
	rs.connSync.Unlock()
	return gnet.Action(rs.onClosed(&Conn{Conn: c}, err))
}

// React handles incoming data from connections
func (rs *RedHub) React(frame []byte, c gnet.Conn) (out []byte, action gnet.Action) {
	rs.connSync.RLock()
	cb, ok := rs.redHubBufMap[c]
	rs.connSync.RUnlock()

	if !ok {
		return resp.AppendError(out, "ERR Client is closed"), gnet.None
	}

	cb.buf.Write(frame)
	cmds, lastbyte, err := resp.ReadCommands(cb.buf.Bytes())
	if err != nil {
		return resp.AppendError(out, "ERR "+err.Error()), gnet.None
	}

	cb.command = append(cb.command, cmds...)
	cb.buf.Reset()

	if len(lastbyte) == 0 {
		for len(cb.command) > 0 {
			cmd := cb.command[0]
			cb.command = cb.command[1:]

			var status Action
			out, status = rs.handler(cmd, out)

			if status == Close {
				return out, gnet.Close
			}
		}
	} else {
		cb.buf.Write(lastbyte)
	}

	return out, gnet.None
}

// ListenAndServe starts the RedHub server
func ListenAndServe(addr string, options Options, rh *RedHub) error {
	serveOptions := gnet.Options{
		Multicore:        options.Multicore,
		LockOSThread:     options.LockOSThread,
		ReadBufferCap:    options.ReadBufferCap,
		LB:               options.LB,
		NumEventLoop:     options.NumEventLoop,
		ReusePort:        options.ReusePort,
		Ticker:           options.Ticker,
		TCPKeepAlive:     options.TCPKeepAlive,
		TCPNoDelay:       options.TCPNoDelay,
		SocketRecvBuffer: options.SocketRecvBuffer,
		SocketSendBuffer: options.SocketSendBuffer,
		Codec:            options.Codec,
	}

	return gnet.Serve(rh, addr, gnet.WithOptions(serveOptions))
}
