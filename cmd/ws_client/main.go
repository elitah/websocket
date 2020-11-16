package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/elitah/fast-io"
	"github.com/elitah/go-socks5"
	"github.com/elitah/utils/errors"
	"github.com/elitah/utils/exepath"
	"github.com/elitah/utils/httpproxy"
	"github.com/elitah/utils/logs"
	"github.com/elitah/utils/udp_relay"
	"github.com/elitah/utils/wait"
	"github.com/elitah/websocket/websocket"

	"github.com/xtaci/smux"
)

var (
	exeDir = exepath.GetExeDir()
)

type proxyAddr struct {
	Proto  string
	Target string
}

func (this *proxyAddr) Network() string {
	return this.Proto
}

func (this *proxyAddr) String() string {
	return this.Target
}

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

func getNetInfo(address string, def ...string) (string, string) {
	//
	if netinfo := strings.SplitN(address, ":", 2); 2 <= len(netinfo) {
		return netinfo[0], netinfo[1]
	}
	//
	if 0 < len(def) {
		return def[0], address
	}
	//
	return "", address
}

func startUDPRelay(dial func(*connAsync) (net.Conn, error), listen, target string) *udp_relay.UDPTransmission {
	//
	var network string
	//
	network, target = getNetInfo(target, "udp4")
	//
	if laddr, err := net.ResolveUDPAddr("udp4", listen); nil == err {
		if _, err := net.ResolveUDPAddr("udp4", target); nil == err {
			if conn, err := net.ListenUDP("udp4", laddr); nil == err {
				if ut := udp_relay.NewUDPTransmission(
					180*time.Second,
					func(src *net.UDPAddr) udp_relay.RelayConn {
						//
						logs.Info("connect: %v => %s", src, target)
						//
						if conn, err := dial(&connAsync{
							ch:      make(chan net.Conn, 1),
							network: network,
							address: target,
						}); nil == err {
							return udp_relay.NewWrapConn(conn, &proxyAddr{
								Proto:  "udp4",
								Target: target,
							})
						} else {
							logs.Warn("failed: %v => %s: %v", src, target, err)
						}
						//
						return nil
					},
					func(p *udp_relay.UDPPacket) {
						if _, err := conn.WriteToUDP(p.Data[:p.Length], p.Address); nil != err {
							logs.Error(err)
						}
					},
				); nil != ut {
					//
					ut.SetErrorHandlerFunc(func(err error) {
						if !errors.IsTimeout(err) {
							logs.Error(err)
						}
					})
					//
					logs.Info("udp proxy service start: %s -> %s", conn.LocalAddr(), target)
					//
					go func() {
						//
						for {
							if p := ut.GetUDPPacket(); nil != p {
								if p.Length, p.Address, err = conn.ReadFromUDP(p.Data[:]); nil == err {
									if ut.Forward(p) {
										continue
									} else {
										logs.Error("forward failed")
									}
								} else {
									logs.Error(err)
								}
								//
								ut.PutUDPPacket(p)
							}
						}
					}()
					//
					return ut
				} else {
					logs.Error("NewUDPTransmission failed")
				}
			} else {
				logs.Error(err)
			}
		} else {
			logs.Error(err)
		}
	} else {
		logs.Error(err)
	}
	//
	return nil
}

func startTCPRelay(dial func(*connAsync) (net.Conn, error), listen, target string) bool {
	//
	var network string
	//
	network, target = getNetInfo(target, "udp4")
	//
	if laddr, err := net.ResolveTCPAddr("tcp4", listen); nil == err {
		if _, err := net.ResolveTCPAddr("tcp4", target); nil == err {
			if ls, err := net.ListenTCP("udp4", laddr); nil == err {
				go func() {
					for {
						if conn, err := ls.AcceptTCP(); nil == err {
							go func(local *net.TCPConn) {
								//
								defer local.Close()
								//
								logs.Info("connect: %v => %s", local, target)
								//
								if remote, err := dial(&connAsync{
									ch:      make(chan net.Conn, 1),
									network: network,
									address: target,
								}); nil == err {
									//
									fast_io.FastCopy(local, remote)
									//
									remote.Close()
								} else {
									logs.Warn("failed: %v => %s: %v", local, target, err)
								}
							}(conn)
						} else {
							break
						}
					}
				}()
				//
				logs.Info("udp proxy service start: %s -> %s", ls.Addr(), target)
				//
				return true
			} else {
				logs.Error(err)
			}
		} else {
			logs.Error(err)
		}
	} else {
		logs.Error(err)
	}
	//
	return false
}

func main() {
	var links uint64

	var point unsafe.Pointer

	var wsurl string
	var ws_ip_v4 string
	var http_proxy_addr string
	var socks5_proxy_addr string
	var udp_proxy string
	var tcp_proxy string

	var ca_path string

	var username, password string

	var cookies string

	var queue_cnt, limit_cnt int

	var udpPorxyList []*udp_relay.UDPTransmission

	flag.StringVar(&wsurl, "w", "", "Your websocket server url")
	flag.StringVar(&ws_ip_v4, "ip", "", "Your websocket server IPv4 address")
	flag.StringVar(&http_proxy_addr, "h", "", "Your http proxy server address")
	flag.StringVar(&socks5_proxy_addr, "s", "", "Your socks5 proxy server address")
	flag.StringVar(&udp_proxy, "u", "", "Your udp proxy server address")
	flag.StringVar(&tcp_proxy, "t", "", "Your tcp proxy server address")

	flag.StringVar(&ca_path, "ca", exeDir+"/rootCA.bin", "Your CA Certificate file path")

	flag.StringVar(&username, "username", "", "Your username")
	flag.StringVar(&password, "password", "", "Your password")

	flag.StringVar(&cookies, "cookies", "", "Your cookies")

	flag.IntVar(&queue_cnt, "queue", 32, "Your queue length")
	flag.IntVar(&limit_cnt, "limit", 4, "Your limit count")

	flag.Parse()

	logs.SetLogger(logs.AdapterConsole, `{"level":99,"color":true}`)
	logs.EnableFuncCallDepth(true)
	logs.SetLogFuncCallDepth(3)
	logs.Async()

	defer logs.Close()

	if "" == wsurl {
		flag.Usage()
		return
	}

	if "" == http_proxy_addr &&
		"" == socks5_proxy_addr &&
		"" == udp_proxy {
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

	dial := func(req *connAsync) (net.Conn, error) {
		var cancel context.CancelFunc
		//
		if nil != req.ctx {
			if _, ok := req.ctx.Deadline(); !ok {
				req.ctx, cancel = context.WithTimeout(req.ctx, 10*time.Second)
			} else {
				req.ctx, cancel = context.WithCancel(req.ctx)
			}
		} else {
			req.ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		}
		//
		defer func() {
			//
			cancel()
			//
			req.Close()
		}()
		//
		request <- req
		//
		select {
		case conn, ok := <-req.ch:
			if ok {
				return conn, nil
			}
		case <-req.ctx.Done():
			return nil, fmt.Errorf("context report done: %s:%s", req.network, req.address)
		}
		//
		return nil, fmt.Errorf("dial failed: %s:%s", req.network, req.address)
	}

	if "" != http_proxy_addr {
		if listener, err := net.Listen("tcp", http_proxy_addr); nil == err {
			//
			logs.Info("http proxy service active at: %s", listener.Addr())
			//
			go func() {
				logs.Error(http.Serve(listener, &httpproxy.HttpProxy{
					DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
						//
						return dial(&connAsync{
							ch:      make(chan net.Conn, 1),
							ctx:     ctx,
							network: network,
							address: address,
						})
					},
				}))
			}()
		} else {
			logs.Error(err)

			return
		}
	}

	if "" != socks5_proxy_addr {
		//
		if listener, err := net.Listen("tcp", socks5_proxy_addr); nil == err {
			//
			logs.Info("socks5 proxy service active at: %s", listener.Addr())
			//
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
					//
					return dial(&connAsync{
						ch:      make(chan net.Conn, 1),
						ctx:     ctx,
						network: network,
						address: address,
					})
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
	}

	// :80@xxx.x1.com:80;:81@xxx.x2.com:81
	if "" != udp_proxy {
		if list := strings.Split(udp_proxy, ";"); 0 < len(list) {
			for _, option := range list {
				if options := strings.Split(option, "@"); 2 <= len(options) {
					if ut := startUDPRelay(dial, options[0], options[1]); nil != ut {
						udpPorxyList = append(udpPorxyList, ut)
					} else {
						logs.Error("unable create udp relay: %s", options)
					}
				} else {
					logs.Error("unable split: %s", option)
				}
			}
		} else {
			logs.Error("unable split: %s", udp_proxy)
		}
	}

	// :80@xxx.x1.com:80;:81@xxx.x2.com:81
	if "" != tcp_proxy {
		if list := strings.Split(tcp_proxy, ";"); 0 < len(list) {
			for _, option := range list {
				if options := strings.Split(option, "@"); 2 <= len(options) {
					if !startTCPRelay(dial, options[0], options[1]) {
						logs.Error("unable create tcp relay: %s", options)
					}
				} else {
					logs.Error("unable split: %s", option)
				}
			}
		} else {
			logs.Error("unable split: %s", udp_proxy)
		}
	}

	ws := websocket.NewClient()

	defer ws.Close()

	if "" != ws_ip_v4 {
		ws.SetNetDial(func(network, address string) (net.Conn, error) {
			//
			_, port, _ := net.SplitHostPort(address)
			//
			if "" != port {
				return net.DialTimeout(network, fmt.Sprintf("%s:%s", ws_ip_v4, port), 5*time.Second)
			}
			//
			return net.DialTimeout(network, address, 5*time.Second)
		})
	}

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

	if "" != username && "" != password {
		if u, err := url.Parse(wsurl); nil == err {
			//
			v := url.Values{}
			//
			v.Set("username", username)
			v.Set("password", password)
			//
			u.RawPath = ""
			u.ForceQuery = false
			u.RawQuery = v.Encode()
			u.Fragment = ""
			//
			wsurl = u.String()
		} else {
			logs.Error(err)

			return
		}
	}

	h := make(http.Header)

	if "" != cookies {
		h.Set("Cookie", cookies)

		for key, values := range h {
			logs.Info("[%s]: %v", key, values)
		}
	}

	reload := make(chan struct{}, 1)

	reload <- struct{}{}

	if err := wait.Signal(
		wait.WithTicket(3, nil),
		wait.WithSelect(func(sig <-chan os.Signal, ticker <-chan time.Time) {
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
											logs.Error("%v: %s:%s", err, req.network, req.address)
										}
									}
								}
								//
								if err := errors.TryCatchPanic(func() {
									reload <- struct{}{}
								}); nil != err {
									logs.Error(err)
								}
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
								//
								ok = false
								//
								logs.Info("Dial: %s", wsurl)
								//
								if conn, err := ws.Dial(wsurl, h); nil == err {
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
						//
						return
					}
				case <-ticker:
					//
					logs.Info("=== links: %d ============================", atomic.LoadUint64(&links))
					logs.Info("=== report(%d) ============================", ws.Len())
					//
					for _, item := range ws.Lists() {
						logs.Info(item)
					}
					//
					logs.Info("=======================================\r\n\r\n")
					//
					for _, item := range udpPorxyList {
						logs.Info("###############################################################")
						logs.Info(item.String())
					}
					//
					logs.Info("=======================================\r\n\r\n")
				}
			}
		}),
	); nil != err {
		logs.Error(err)
	}
}
