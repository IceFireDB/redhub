<!-- <p align="center">
<img 
    src="" 
    width="336" height="75" border="0" alt="REDHUB">
<br>
<a href=""><img src="https://img.shields.io/badge/api-reference-blue.svg?style=flat-square" alt="GoDoc"></a>
</p>

<p align="center">Redis compatible server framework with RawEpoll model</p> -->

Features
--------
- Create a Redis compatible server with RawEpoll model in Go

Installing
----------

```
go get -u github.com/IceFireDB/redhub
```

Example
-------

Here's a full example of a Redis clone that accepts:

- SET key value
- GET key
- DEL key
- PING
- QUIT

You can run this example from a terminal:

```sh
go run example/server.go
```

```go
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
	"github.com/Jchicode/redhub/pkg/resp"
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
	option := redhub.Options{}
	option.Multicore = multicore
	err := redhub.ListendAndServe(addr,
		func(c *redhub.Conn) (out []byte, action redhub.Action) {
			return
		},
		func(c *redhub.Conn, err error) (action redhub.Action) {
			return
		},
		func(c *redhub.Conn, cmd resp.Command) (out []byte) {
			switch strings.ToLower(string(cmd.Args[0])) {
			default:
				out = resp.AppendError(out, "ERR unknown command '"+string(cmd.Args[0])+"'")
			case "ping":
				out = resp.AppendString(out, "PONG")
			case "quit":
				out = resp.AppendString(out, "OK")
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
			return
		},
		option,
	)
	if err != nil {
		log.Fatal(err)
	}
}
```

Benchmarks
----------



License
-------
Redhub source code is available under the Apache 2.0 [License](/LICENSE).
