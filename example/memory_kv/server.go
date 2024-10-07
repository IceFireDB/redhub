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
	// Define a mutex and a map to store data
	var mu sync.RWMutex
	var items = make(map[string][]byte)

	// Define command-line arguments
	var network string
	var addr string
	var multicore bool
	var reusePort bool
	var pprofDebug bool
	var pprofAddr string

	// Parse command-line arguments
	flag.StringVar(&network, "network", "tcp", "server network (default \"tcp\")")
	flag.StringVar(&addr, "addr", "127.0.0.1:6380", "server address (default \":6380\")")
	flag.BoolVar(&multicore, "multicore", true, "enable multicore support")
	flag.BoolVar(&reusePort, "reusePort", false, "enable port reuse")
	flag.BoolVar(&pprofDebug, "pprofDebug", false, "enable pprof debugging")
	flag.StringVar(&pprofAddr, "pprofAddr", ":8888", "pprof address")
	flag.Parse()

	// Start pprof server if debugging is enabled
	if pprofDebug {
		go func() {
			http.ListenAndServe(pprofAddr, nil)
		}()
	}

	// Create the protocol address string
	protoAddr := fmt.Sprintf("%s://%s", network, addr)

	// Define RedHub options
	option := redhub.Options{
		Multicore: multicore,
		ReusePort: reusePort,
	}

	// Create a new RedHub instance with custom handlers
	rh := redhub.NewRedHub(
		// Connection initialization handler
		func(c *redhub.Conn) (out []byte, action redhub.Action) {
			return
		},
		// Connection error handler
		func(c *redhub.Conn, err error) (action redhub.Action) {
			return
		},
		// Command handler
		func(cmd resp.Command, out []byte) ([]byte, redhub.Action) {
			var status redhub.Action
			switch strings.ToLower(string(cmd.Args[0])) {
			default:
				// Handle unknown commands
				out = resp.AppendError(out, "ERR unknown command '"+string(cmd.Args[0])+"'")
			case "ping":
				// Handle PING command
				out = resp.AppendString(out, "PONG")
			case "quit":
				// Handle QUIT command
				out = resp.AppendString(out, "OK")
				status = redhub.Close
			case "set":
				// Handle SET command
				if len(cmd.Args) != 3 {
					out = resp.AppendError(out, "ERR wrong number of arguments for '"+string(cmd.Args[0])+"' command")
					break
				}
				mu.Lock()
				items[string(cmd.Args[1])] = cmd.Args[2]
				mu.Unlock()
				out = resp.AppendString(out, "OK")
			case "get":
				// Handle GET command
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
				// Handle DEL command
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
				// Handle CONFIG command (for redis-benchmark compatibility)
				out = resp.AppendArray(out, 2)
				out = resp.AppendBulk(out, cmd.Args[2])
				out = resp.AppendBulkString(out, "")
			}
			return out, status
		},
	)

	// Log the server start
	log.Printf("started redhub server at %s", addr)

	// Start the RedHub server
	err := redhub.ListenAndServe(protoAddr, option, rh)
	if err != nil {
		log.Fatal(err)
	}
}
