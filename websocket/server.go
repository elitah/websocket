package websocket

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/elitah/utils/logs"
	"github.com/gorilla/websocket"
)

type Server struct {
	sync.Mutex

	sync.Pool // for websocket.Upgrader::WriteBufferPool

	enc *JsonEncoder

	list map[string]*Conn

	upgrader *websocket.Upgrader

	handlerMsg func(*Conn, string)

	handlerAuth          func(*Values, interface{}) bool
	handlerRawConnection func(*Conn)
	handlerConnected     func(*Conn) bool
}

func NewServer(fn func(*Conn, string)) *Server {
	srv := &Server{
		enc: NewJsonEncoder(),

		list: make(map[string]*Conn),

		handlerMsg: fn,
	}
	srv.upgrader = &websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,

		ReadBufferSize:  1024,
		WriteBufferSize: 1024,

		WriteBufferPool: srv,

		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	return srv
}

func (this *Server) Close() {
	var list []*Conn

	this.Lock()

	// 获取连接列表
	for _, c := range this.list {
		list = append(list, c)
	}

	this.Unlock()

	for _, c := range list {
		logs.Info("Start to close: %s", c.ID())
		c.Close()
		logs.Info("Close done: %s", c.ID())
	}
}

func (this *Server) AddAuthHandler(fn func(*Values, interface{}) bool) {
	if nil != fn {
		this.handlerAuth = fn
	}
}

func (this *Server) AddRawConnectionHandler(fn func(*Conn)) {
	if nil != fn {
		this.handlerRawConnection = fn
	}
}

func (this *Server) AddConnectedHandler(fn func(*Conn) bool) {
	if nil != fn {
		this.handlerConnected = fn
	}
}

func (this *Server) Upgrade(w http.ResponseWriter, r *http.Request, args ...interface{}) {
	//
	this.UpgradeWithValues(w, r, &Values{}, args...)
}

func (this *Server) UpgradeWithValues(w http.ResponseWriter, r *http.Request, v *Values, args ...interface{}) {
	// 判断是否是websocket连接
	if websocket.IsWebSocketUpgrade(r) {
		// 检查websocket模块是否开启
		if nil != this.upgrader {
			// 检查是否需要授权
			if nil == this.handlerAuth || 0 == len(args) || this.handlerAuth(v, args[0]) {
				// 调用websocket模块初始化连接
				if rawconn, err := this.upgrader.Upgrade(w, r, nil); nil == err {
					// 创建连接对象
					if conn, err := newConn(rawconn, v); nil == err {
						// 加入列表
						if this.listAdd(conn.ID(), conn) {
							// 关闭回调
							conn.AddCloseHandler(this.listDelete)
							// 检查是否启动原始连接模式
							if nil != this.handlerRawConnection {
								this.handlerRawConnection(conn)
							} else {
								// 检查并调用连接上线通知
								if nil == this.handlerConnected || this.handlerConnected(conn) {
									// 同步消息处理
									conn.HandleConn(this.handlerMsg)
								}
							}
						}
						// 关闭
						conn.Close()
					} else {
						logs.Error(err)
					}
					// 关闭
					rawconn.Close()
					//
					return
				} else {
					// 错误输出
					logs.Error(err)
					// 内部错误
					w.WriteHeader(http.StatusInternalServerError)
				}
			} else {
				// 授权错误
				w.WriteHeader(http.StatusUnauthorized)
			}
		} else {
			// websocket模块未初始化
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	} else {
		// 不是websocket请求
		w.WriteHeader(http.StatusNotAcceptable)
	}
}

func (this *Server) BroadCastData(data []byte) error {
	if 0 < len(data) {
		this.Lock()

		for _, conn := range this.list {
			conn.WriteMessage(websocket.TextMessage, data)
		}

		this.Unlock()
	}

	return nil
}

func (this *Server) BroadCastString(msg string) error {
	if "" != msg {
		return this.BroadCastData([]byte(msg))
	} else {
		return ErrBadInput
	}
}

func (this *Server) BroadCastJSON(v interface{}) error {
	if nil != v {
		if nil != this.enc {
			this.enc.Lock()
			defer this.enc.Unlock()
			if err := this.enc.Encode(v); nil == err {
				if data := this.enc.Bytes(); nil != data {
					return this.BroadCastData(data)
				} else {
					return ErrEmptyData
				}
			} else {
				return err
			}
		} else {
			if data, err := json.Marshal(v); nil == err {
				return this.BroadCastData(data)
			} else {
				return err
			}
		}
	} else {
		return ErrBadInput
	}
}

func (this *Server) Lists() []string {
	var list []string

	this.Lock()

	for key, _ := range this.list {
		list = append(list, key)
	}

	this.Unlock()

	return list
}

func (this *Server) GetConnByID(id string) *Conn {
	this.Lock()
	defer this.Unlock()

	if conn, ok := this.list[id]; ok {
		return conn
	}

	return nil
}

func (this *Server) listAdd(id string, conn *Conn) bool {
	if "" != id && nil != conn {
		this.Lock()
		defer this.Unlock()

		// 检查列表是否重名
		if _, ok := this.list[id]; !ok {
			// 加入列表
			this.list[id] = conn
			// 返回成功
			return true
		}
	}

	return false
}

func (this *Server) listDelete(id string) {
	if "" != id {
		this.Lock()
		defer this.Unlock()

		// 移除列表
		delete(this.list, id)
	}
}
