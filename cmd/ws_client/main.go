package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/elitah/fast-io"
	"github.com/elitah/go-socks5"
	"github.com/elitah/utils/exepath"
	"github.com/elitah/utils/logs"
	"github.com/elitah/websocket/websocket"
	"github.com/xtaci/smux"
)

var (
	exeDir = exepath.GetExeDir()
)

type cheatDNSResolver struct {
}

func (this *cheatDNSResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	return ctx, nil, nil
}

func (this *cheatDNSResolver) Rewrite(ctx context.Context, request *socks5.Request) (context.Context, *socks5.AddrSpec) {
	return ctx, request.DestAddr
}

type connAsync struct {
	flag    uint32
	ch      chan net.Conn
	ctx     context.Context
	network string
	address string
}

func (this *connAsync) Close() {
	if atomic.CompareAndSwapUint32(&this.flag, 0x0, 0x1) {
		close(this.ch)
	}
}

func (this *connAsync) Send(conn net.Conn) {
	if 0x0 == atomic.LoadUint32(&this.flag) {
		defer func() {
			recover()
		}()
		this.ch <- conn
	} else {
		conn.Close()
	}
}

func (this *connAsync) connectTo(s *smux.Session) error {
	if nil != this.ch {
		if nil != s {
			if conn, err := s.OpenStream(); nil == err {
				var buffer [1024]byte
				//
				logs.Info(this.network, this.address)
				//
				fmt.Fprintf(conn, "%s:%s", this.network, this.address)
				//
				conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				//
				if n, _err := conn.Read(buffer[:]); nil == _err {
					if 0 < n {
						if 2 == n && "ok" == string(buffer[:n]) {
							//
							conn.SetReadDeadline(time.Time{})
							//
							this.Send(conn)
							//
							return nil
						} else if 6 == n && "failed" == string(buffer[:n]) {
							err = nil
						}
					} else {
						err = fmt.Errorf("short read")
					}
				} else {
					err = _err
				}
				//
				conn.Close()
				//
				return err
			} else {
				return err
			}
		} else {
			return fmt.Errorf("no session")
		}
	} else {
		return fmt.Errorf("no channel")
	}
}

func load(p *unsafe.Pointer) *smux.Session {
	if nil != p {
		if result := atomic.LoadPointer(p); nil != result {
			_result := (*smux.Session)(result)
			return _result
		}
	}
	return nil
}

func store(p *unsafe.Pointer, s *smux.Session) {
	if nil != p {
		atomic.StorePointer(p, unsafe.Pointer(s))
	}
}

func main() {
	var links uint64

	var point unsafe.Pointer

	var wsurl string
	var socks5_addr string

	var ca_path string

	var queue_cnt, limit_cnt int

	flag.StringVar(&wsurl, "u", "", "Your websocket server url")
	flag.StringVar(&socks5_addr, "s", ":1080", "Your socks5 server address")

	flag.StringVar(&ca_path, "ca", exeDir+"/rootCA.bin", "Your CA Certificate file path")

	flag.IntVar(&queue_cnt, "queue", 32, "Your queue length")
	flag.IntVar(&limit_cnt, "limit", 4, "Your limit count")

	flag.Parse()

	logs.SetLogger(logs.AdapterConsole, `{"level":99,"color":true}`)
	logs.EnableFuncCallDepth(true)
	logs.SetLogFuncCallDepth(3)
	logs.Async()

	defer logs.Close()

	if "" == wsurl || "" == socks5_addr {
		flag.Usage()
		return
	}

	if !strings.HasPrefix(wsurl, "ws://") && !strings.HasPrefix(wsurl, "wss://") {
		//
		logs.Warn("The supplied parameter is not valid: %s", wsurl)
		//
		flag.Usage()
		//
		return
	}

	if 1 > queue_cnt {
		queue_cnt = 1
	}

	if 1 > limit_cnt {
		limit_cnt = 1
	}

	resolver := &cheatDNSResolver{}

	request := make(chan *connAsync, queue_cnt)
	limit := make(chan struct{}, limit_cnt)

	if listener, err := net.Listen("tcp", socks5_addr); nil == err {
		if server, err := socks5.New(&socks5.Config{
			Resolver: resolver,
			Rewriter: resolver,
			Proxy: func(local, remote net.Conn) error {
				//
				atomic.AddUint64(&links, 1)
				//
				fast_io.FastCopy(local, remote)
				//
				atomic.AddUint64(&links, ^uint64(0))
				//
				return nil
			},
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				var cancel func()

				if _, ok := ctx.Deadline(); !ok {
					ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
				} else {
					ctx, cancel = context.WithCancel(ctx)
				}

				defer cancel()

				req := &connAsync{}

				req.ch = make(chan net.Conn, 1)
				req.ctx = ctx
				req.network = network
				req.address = address

				request <- req

				defer req.Close()

				select {
				case conn, ok := <-req.ch:
					if ok {
						return conn, nil
					}
				case <-ctx.Done():
					return nil, fmt.Errorf("context report done")
				}
				return nil, fmt.Errorf("dial failed")
			},
		}); nil == err {
			go func() {
				logs.Error(server.Serve(listener))
			}()
		} else {
			logs.Error(err)

			return
		}
	} else {
		logs.Error(err)

		return
	}

	ws := websocket.NewClient()

	defer ws.Close()

	if strings.HasPrefix(wsurl, "wss://") && "" != ca_path {
		//
		if info, err := os.Stat(ca_path); nil == err {
			if 0 < info.Size() {
				if data, err := ioutil.ReadFile(ca_path); nil == err {
					pool := x509.NewCertPool()
					if pool.AppendCertsFromPEM(data) {
						ws.SetTLSConfig(&tls.Config{
							RootCAs:            pool,
							InsecureSkipVerify: false,
						})
					}
				}
			}
		}
	}

	sig := make(chan os.Signal, 1)

	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	reload := make(chan struct{})

	ticker := time.NewTicker(3 * time.Second)

	defer ticker.Stop()

	for {
		select {
		case req, ok := <-request:
			if ok {
				go func() {
					limit <- struct{}{}
					//
					defer func() {
						//
						<-limit
						//
						req.Close()
					}()
					//
					for {
						//
						if s := load(&point); nil != s {
							// 如果通道未关闭
							if !s.IsClosed() {
								if err := req.connectTo(s); nil == err {
									return
								} else {
									logs.Error(err)
								}
							}
						}
						//
						reload <- struct{}{}
						//
						select {
						case <-req.ctx.Done():
							return
						case <-time.After(1 * time.Second):
						}
					}
				}()
			}
		case _, ok := <-reload:
			if ok {
				for ok {
					select {
					case <-reload:
					default:
						ok = false
						//
						if conn, err := ws.Dial(wsurl); nil == err {
							// 创建smux
							if session, err := smux.Client(conn, nil); nil == err {
								// 最后连接数
								var last, current int = 1, 0
								// 启动闲置检测
								conn.StartIdlaCheck(30*time.Second, func(before, now uint64) {
									//
									current = session.NumStreams()
									//
									logs.Info(last, current)
									//
									if before == now || (0 == last && 0 == current) {
										session.Close()
									} else {
										last = current
									}
								})
								// 存储
								store(&point, session)
							} else {
								// 关闭
								conn.Close()
								// 错误输出
								logs.Error(err)
							}
						} else {
							logs.Error(err)
						}
					}
				}
			} else {
				return
			}
		case s, ok := <-sig:
			if ok {
				//
				logs.Info("exit by %v", s)
				//
				close(reload)
				close(sig)
			}
		case <-ticker.C:
			//
			logs.Info("=== links: %d ============================", atomic.LoadUint64(&links))
			logs.Info("=== report(%d) ============================", ws.Len())
			//
			for _, item := range ws.Lists() {
				logs.Info(item)
			}
			//
			logs.Info("=======================================\r\n\r\n")
		}
	}
}
