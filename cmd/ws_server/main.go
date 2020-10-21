package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/elitah/fast-io"
	"github.com/elitah/utils/logs"
	"github.com/elitah/websocket/websocket"
	"github.com/xtaci/smux"
)

var (
	links   uint64
	tunnels uint64
)

func handshake(conn *smux.Stream) net.Conn {
	var buffer [1024]byte

	if n, err := conn.Read(buffer[:]); nil == err {
		if 0 < n {
			//
			logs.Info(string(buffer[:n]))
			//
			if ss := strings.SplitN(string(buffer[:n]), ":", 2); 2 <= len(ss) {
				//
				switch ss[0] {
				case "tcp", "tcp4", "tcp6":
					if _conn, err := net.DialTimeout(ss[0], ss[1], 5*time.Second); nil == err {
						return _conn
					} else {
						logs.Error(err)
					}
				case "udp", "udp4", "udp6":
					if addr, err := net.ResolveUDPAddr(ss[0], ss[1]); nil == err {
						if _conn, err := net.DialUDP(ss[0], nil, addr); nil == err {
							return _conn
						} else {
							logs.Error(err)
						}
					} else {
						logs.Error("spilt failed")
					}
				default:
					logs.Warn("unsupport protocol: %s", ss[0])
				}
			} else {
				logs.Error("spilt failed")
			}
		} else {
			logs.Error("short read")
		}
	} else {
		logs.Error(err)
	}

	return nil
}

func handleConn(local *smux.Stream) {
	if remote := handshake(local); nil != remote {
		//
		fmt.Fprint(local, "ok")
		//
		atomic.AddUint64(&links, 1)
		//
		fast_io.FastCopy(local, remote)
		//
		atomic.AddUint64(&links, ^uint64(0))
	} else {
		//
		fmt.Fprint(local, "failed")
		//
		local.Close()
	}
}

func main() {
	var listen_address string

	flag.StringVar(&listen_address, "l", ":80", "Your listen address")

	flag.Parse()

	logs.SetLogger(logs.AdapterConsole, `{"level":99,"color":true}`)
	logs.EnableFuncCallDepth(true)
	logs.SetLogFuncCallDepth(3)
	logs.Async()

	defer logs.Close()

	if "" == listen_address {
		flag.Usage()
		return
	}

	ws := websocket.NewServer(nil)

	ws.AddRawConnectionHandler(func(conn *websocket.Conn) {
		//
		atomic.AddUint64(&tunnels, 1)
		//
		logs.Info("new client: %s", conn.RemoteAddr().String())
		//
		if session, err := smux.Server(conn, nil); nil == err {
			for {
				if stream, err := session.AcceptStream(); nil == err {
					go handleConn(stream)
				} else {
					//
					logs.Info("smux server exit")
					//
					if !errors.As(err, &io.EOF) {
						logs.Error(err)
					}
					//
					break
				}
			}
		} else {
			//
			logs.Info("unable start smux server")
			//
			logs.Error(err)
		}
		//
		logs.Info("release client: %s", conn.RemoteAddr().String())
		//
		atomic.AddUint64(&tunnels, ^uint64(0))
	})

	go func() {
		http.ListenAndServe(listen_address, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				fmt.Fprint(w, "hello world")
				return
			case "/ws":
				ws.Upgrade(w, r)
				return
			}
			http.NotFound(w, r)
		}))
	}()

	sig := make(chan os.Signal, 1)

	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	ticker := time.NewTicker(3 * time.Second)

	defer ticker.Stop()

	for {
		select {
		case s, ok := <-sig:
			if ok {
				//
				logs.Info("exit by %v", s)
				//
				close(sig)
			}
			return
		case <-ticker.C:
			//
			logs.Info("=== links: %d ============================", atomic.LoadUint64(&links))
			logs.Info("=== tunnels: %d ============================\r\n\r\n", atomic.LoadUint64(&tunnels))
		}
	}
}
