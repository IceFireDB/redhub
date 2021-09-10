package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"

	"net/http"
	_ "net/http/pprof"

	"github.com/Jchicode/redhub"
	"github.com/panjf2000/gnet"
)

func main() {
	var mu sync.RWMutex
	var items = make(map[string][]byte)
	var port int
	var multicore bool
	var pprofDebug bool
	var pprofAddr string
	flag.IntVar(&port, "port", 6382, "server port")
	flag.BoolVar(&multicore, "multicore", true, "multicore")
	flag.BoolVar(&pprofDebug, "pprofDebug", false, "open pprof")
	flag.StringVar(&pprofAddr, "pprofAddr", ":8888", "pprof address")
	flag.Parse()
	if pprofDebug {
		go func() {
			http.ListenAndServe(pprofAddr, nil)
		}()
	}

	addr := fmt.Sprintf("tcp://:%d", port)
	option := gnet.Options{
		Multicore: multicore,
		//ReusePort: true,
	}
	err := redhub.ListendAndServe(addr,
		func(c gnet.Conn) (out []byte, action gnet.Action) {
			return
		},
		func(c gnet.Conn, err error) (action gnet.Action) {
			return
		},
		func(c gnet.Conn, cmd redhub.Command) (out []byte) {
			switch strings.ToLower(string(cmd.Args[0])) {
			default:
				out = redhub.AppendError(out, "ERR unknown command '"+string(cmd.Args[0])+"'")
			case "ping":
				out = redhub.AppendString(out, "PONG")
			case "quit":
				out = redhub.AppendString(out, "OK")
			case "set":
				if len(cmd.Args) != 3 {
					out = redhub.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.Lock()
				items[string(cmd.Args[1])] = cmd.Args[2]
				mu.Unlock()
				out = redhub.AppendString(out, "OK")
			case "get":
				if len(cmd.Args) != 2 {
					out = redhub.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.RLock()
				val, ok := items[string(cmd.Args[1])]
				mu.RUnlock()
				if !ok {
					out = redhub.AppendNull(out)
				} else {
					out = redhub.AppendBulk(out, val)
				}
			case "del":
				if len(cmd.Args) != 2 {
					out = redhub.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.Lock()
				_, ok := items[string(cmd.Args[1])]
				delete(items, string(cmd.Args[1]))
				mu.Unlock()
				if !ok {
					out = redhub.AppendInt(out, 0)
				} else {
					out = redhub.AppendInt(out, 1)
				}
			case "config":
				// This simple (blank) response is only here to allow for the
				// redis-benchmark command to work with this example.
				out = redhub.AppendArray(out, 2)
				out = redhub.AppendBulk(out, cmd.Args[2])
				out = redhub.AppendBulkString(out, "")
			}
			return
		},
		option,
	)
	if err != nil {
		log.Fatal(err)
	}
}
