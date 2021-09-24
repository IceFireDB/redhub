package redhub

import (
	"bytes"
	"sync"

	"github.com/IceFireDB/redhub/pkg/resp"
	"github.com/panjf2000/gnet"
)

const (
	// None indicates that no action should occur following an event.
	None Action = iota

	// Close closes the connection.
	Close

	// Shutdown shutdowns the server.
	Shutdown
)

type Conn struct {
	gnet.Conn
}

type Action int
type Options struct {
	gnet.Options
}

func NewRedHub(
	onOpened func(c *Conn) (out []byte, action Action),
	onClosed func(c *Conn, err error) (action Action),
	handler func(cmd resp.Command, out []byte) ([]byte, Action),
) *redHub {
	return &redHub{
		redHubBufMap: make(map[gnet.Conn]*connBuffer),
		connSync:     sync.RWMutex{},
		onOpened:     onOpened,
		onClosed:     onClosed,
		handler:      handler,
	}
}

type redHub struct {
	*gnet.EventServer
	onOpened     func(c *Conn) (out []byte, action Action)
	onClosed     func(c *Conn, err error) (action Action)
	handler      func(cmd resp.Command, out []byte) ([]byte, Action)
	redHubBufMap map[gnet.Conn]*connBuffer
	connSync     sync.RWMutex
}

type connBuffer struct {
	buf     bytes.Buffer
	command []resp.Command
}

func (rs *redHub) OnOpened(c gnet.Conn) (out []byte, action gnet.Action) {
	rs.connSync.Lock()
	defer rs.connSync.Unlock()
	rs.redHubBufMap[c] = new(connBuffer)
	rs.onOpened(&Conn{Conn: c})
	return
}

func (rs *redHub) OnClosed(c gnet.Conn, err error) (action gnet.Action) {
	rs.connSync.Lock()
	defer rs.connSync.Unlock()
	delete(rs.redHubBufMap, c)
	rs.onClosed(&Conn{Conn: c}, err)
	return
}

func (rs *redHub) React(frame []byte, c gnet.Conn) (out []byte, action gnet.Action) {
	rs.connSync.RLock()
	defer rs.connSync.RUnlock()
	cb, ok := rs.redHubBufMap[c]
	if !ok {
		out = resp.AppendError(out, "ERR Client is closed")
		return
	}
	cb.buf.Write(frame)
	cmds, lastbyte, err := resp.ReadCommands(cb.buf.Bytes())
	if err != nil {
		out = resp.AppendError(out, "ERR "+err.Error())
		return
	}
	cb.command = append(cb.command, cmds...)
	cb.buf.Reset()
	if len(lastbyte) == 0 {
		var status Action
		for len(cb.command) > 0 {
			cmd := cb.command[0]
			if len(cb.command) == 1 {
				cb.command = nil
			} else {
				cb.command = cb.command[1:]
			}
			out, status = rs.handler(cmd, out)
			switch status {
			case Close:
				action = gnet.Close
			}
		}
	} else {
		cb.buf.Write(lastbyte)
	}
	return
}

func ListendAndServe(addr string, options Options, rh *redHub) error {
	return gnet.Serve(rh, addr, gnet.WithOptions(options.Options))
}
