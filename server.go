package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/panjf2000/gnet"
)

var mu sync.RWMutex
var items = make(map[string][]byte)
var iceConn map[gnet.Conn]*connBuffer
var connSync sync.RWMutex

type redisServer struct {
	*gnet.EventServer
}

type connBuffer struct {
	mu      sync.RWMutex
	buf     bytes.Buffer
	command []Command
}

func (rs *redisServer) OnOpened(c gnet.Conn) (out []byte, action gnet.Action) {
	connSync.Lock()
	iceConn[c] = new(connBuffer)
	connSync.Unlock()
	return
}
func (es *redisServer) OnClosed(c gnet.Conn, err error) (action gnet.Action) {
	connSync.Lock()
	delete(iceConn, c)
	connSync.Unlock()
	return
}

func (rs *redisServer) React(frame []byte, c gnet.Conn) (out []byte, action gnet.Action) {
	connSync.Lock()
	cb, ok := iceConn[c]
	if !ok {
		cb = new(connBuffer)
		iceConn[c] = cb
	}
	connSync.Unlock()

	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.buf.Write(frame)
	cmds, lastbyte, err := Parse(cb.buf.Bytes())
	cb.command = append(cb.command, cmds...)
	cb.buf.Reset()
	cb.buf.Write(lastbyte)
	if err != nil {
		out = AppendError(out, "ERR "+err.Error())
	}
	if len(lastbyte) == 0 {
		for len(cb.command) > 0 {
			cmd := cb.command[0]
			if len(cb.command) == 1 {
				cb.command = nil
			} else {
				cb.command = cb.command[1:]
			}
			switch strings.ToLower(string(cmd.Args[0])) {
			default:
				log.Println("parse err")
				out = AppendError(out, "ERR unknown command '"+string(cmd.Args[0])+"'")
			case "set":
				if len(cmd.Args) != 3 {
					fmt.Println(string(out))
					out = AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.Lock()
				items[string(cmd.Args[1])] = cmd.Args[2]
				mu.Unlock()
				out = AppendString(out, "OK")
			case "get":
				if len(cmd.Args) != 2 {
					out = AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.RLock()
				val, ok := items[string(cmd.Args[1])]
				mu.RUnlock()
				if !ok {
					out = AppendNull(out)
				} else {
					out = AppendBulk(out, val)
				}
			}
		}
	}

	return
}

func init() {
	iceConn = make(map[gnet.Conn]*connBuffer)
}

func main() {
	var port int
	var multicore bool
	flag.IntVar(&port, "port", 6382, "server port")
	flag.BoolVar(&multicore, "multicore", true, "multicore")
	flag.Parse()
	addr := fmt.Sprintf("tcp://:%d", port)
	//codec := &RedisCodec{}
	err := gnet.Serve(&redisServer{}, addr, gnet.WithMulticore(multicore), gnet.WithTCPKeepAlive(time.Minute*5)) //, gnet.WithCodec(codec))
	if err != nil {
		panic(err)
	}
}
