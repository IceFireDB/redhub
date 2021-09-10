package redhub

import (
	"bytes"
	"sync"

	"github.com/panjf2000/gnet"
)

var iceConn map[gnet.Conn]*connBuffer
var connSync sync.RWMutex

var mu sync.RWMutex
var items = make(map[string][]byte)

type redisServer struct {
	*gnet.EventServer
	onOpened func(c gnet.Conn) (out []byte, action gnet.Action)
	onClosed func(c gnet.Conn, err error) (action gnet.Action)
	handler  func(c gnet.Conn, cmd Command) []byte
}

type connBuffer struct {
	buf     bytes.Buffer
	command []Command
}

func (rs *redisServer) OnOpened(c gnet.Conn) (out []byte, action gnet.Action) {
	connSync.Lock()
	defer connSync.Unlock()
	iceConn[c] = new(connBuffer)
	rs.onOpened(c)

	return
}

func (rs *redisServer) OnClosed(c gnet.Conn, err error) (action gnet.Action) {
	connSync.Lock()
	defer connSync.Unlock()
	delete(iceConn, c)
	rs.onClosed(c, err)
	return
}

func (rs *redisServer) React(frame []byte, c gnet.Conn) (out []byte, action gnet.Action) {
	connSync.RLock()
	defer connSync.RUnlock()
	cb, ok := iceConn[c]
	if !ok {
		out = AppendError(out, "ERR Client is closed")
		return
	}

	cb.buf.Write(frame)
	cmds, lastbyte, err := Parse(cb.buf.Bytes())
	cb.command = append(cb.command, cmds...)
	cb.buf.Reset()
	cb.buf.Write(lastbyte)
	if err != nil {
		out = AppendError(out, "ERR "+err.Error())
		return
	}
	if len(lastbyte) == 0 {
		for len(cb.command) > 0 {
			cmd := cb.command[0]
			if len(cb.command) == 1 {
				cb.command = nil
			} else {
				cb.command = cb.command[1:]
			}
			out = append(out, rs.handler(c, cmd)...)
		}
	}
	return
}

func init() {
	iceConn = make(map[gnet.Conn]*connBuffer)
}

func ListendAndServe(addr string,
	onOpened func(c gnet.Conn) (out []byte, action gnet.Action),
	onClosed func(c gnet.Conn, err error) (action gnet.Action),
	handler func(c gnet.Conn, cmd Command) []byte,
	options gnet.Options,
) error {
	rs := &redisServer{
		onOpened: onOpened,
		onClosed: onClosed,
		handler:  handler,
	}
	return gnet.Serve(rs, addr, gnet.WithOptions(options))
}
