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
	"github.com/Jchicode/redhub/pkg/redcon"
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
		func(c gnet.Conn, cmd redcon.Command) (out []byte) {
			switch strings.ToLower(string(cmd.Args[0])) {
			default:
				out = redcon.AppendError(out, "ERR unknown command '"+string(cmd.Args[0])+"'")
			case "ping":
				out = redcon.AppendString(out, "PONG")
			case "quit":
				out = redcon.AppendString(out, "OK")
			case "set":
				if len(cmd.Args) != 3 {
					out = redcon.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.Lock()
				items[string(cmd.Args[1])] = cmd.Args[2]
				mu.Unlock()
				out = redcon.AppendString(out, "OK")
			case "get":
				if len(cmd.Args) != 2 {
					out = redcon.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.RLock()
				val, ok := items[string(cmd.Args[1])]
				mu.RUnlock()
				if !ok {
					out = redcon.AppendNull(out)
				} else {
					out = redcon.AppendBulk(out, val)
				}
			case "del":
				if len(cmd.Args) != 2 {
					out = redcon.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.Lock()
				_, ok := items[string(cmd.Args[1])]
				delete(items, string(cmd.Args[1]))
				mu.Unlock()
				if !ok {
					out = redcon.AppendInt(out, 0)
				} else {
					out = redcon.AppendInt(out, 1)
				}
			case "config":
				// This simple (blank) response is only here to allow for the
				// redis-benchmark command to work with this example.
				out = redcon.AppendArray(out, 2)
				out = redcon.AppendBulk(out, cmd.Args[2])
				out = redcon.AppendBulkString(out, "")
			}
			return
		},
		option,
	)
	if err != nil {
		log.Fatal(err)
	}
}
