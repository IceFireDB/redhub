package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"

	"net/http"
	_ "net/http/pprof"

	"github.com/IceFireDB/redhub"
	"github.com/IceFireDB/redhub/pkg/resp"
)

func main() {
	var mu sync.RWMutex
	var items = make(map[string][]byte)
	var network string
	var addr string
	var multicore bool
	var reusePort bool
	var pprofDebug bool
	var pprofAddr string
	flag.StringVar(&network, "network", "tcp", "server network (default \"tcp\")")
	flag.StringVar(&addr, "addr", "127.0.0.1:6380", "server addr (default \":6380\")")
	flag.BoolVar(&multicore, "multicore", true, "multicore")
	flag.BoolVar(&reusePort, "reusePort", false, "reusePort")
	flag.BoolVar(&pprofDebug, "pprofDebug", false, "open pprof")
	flag.StringVar(&pprofAddr, "pprofAddr", ":8888", "pprof address")
	flag.Parse()
	if pprofDebug {
		go func() {
			http.ListenAndServe(pprofAddr, nil)
		}()
	}

	protoAddr := fmt.Sprintf("%s://%s", network, addr)
	option := redhub.Options{
		Multicore: multicore,
		ReusePort: reusePort,
	}

	rh := redhub.NewRedHub(
		func(c *redhub.Conn) (out []byte, action redhub.Action) {
			return
		},
		func(c *redhub.Conn, err error) (action redhub.Action) {
			return
		},
		func(cmd resp.Command, out []byte) ([]byte, redhub.Action) {
			var status redhub.Action
			switch strings.ToLower(string(cmd.Args[0])) {
			default:
				out = resp.AppendError(out, "ERR unknown command '"+string(cmd.Args[0])+"'")
			case "ping":
				out = resp.AppendString(out, "PONG")
			case "quit":
				out = resp.AppendString(out, "OK")
				status = redhub.Close
			case "set":
				if len(cmd.Args) != 3 {
					out = resp.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.Lock()
				items[string(cmd.Args[1])] = cmd.Args[2]
				mu.Unlock()
				out = resp.AppendString(out, "OK")
			case "get":
				if len(cmd.Args) != 2 {
					out = resp.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.RLock()
				val, ok := items[string(cmd.Args[1])]
				mu.RUnlock()
				if !ok {
					out = resp.AppendNull(out)
				} else {
					out = resp.AppendBulk(out, val)
				}
			case "del":
				if len(cmd.Args) != 2 {
					out = resp.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.Lock()
				_, ok := items[string(cmd.Args[1])]
				delete(items, string(cmd.Args[1]))
				mu.Unlock()
				if !ok {
					out = resp.AppendInt(out, 0)
				} else {
					out = resp.AppendInt(out, 1)
				}
			case "config":
				// This simple (blank) response is only here to allow for the
				// redis-benchmark command to work with this example.
				out = resp.AppendArray(out, 2)
				out = resp.AppendBulk(out, cmd.Args[2])
				out = resp.AppendBulkString(out, "")
			}
			return out, status
		},
	)
	log.Printf("started redhub server at %s", addr)
	err := redhub.ListendAndServe(protoAddr, option, rh)
	if err != nil {
		log.Fatal(err)
	}
}
