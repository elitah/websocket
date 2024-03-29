package websocket

import (
	"bytes"
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/elitah/utils/logs"

	"github.com/gorilla/websocket"
)

const (
	WebsocketFlagClosed = iota
	WebsocketFlagIdle

	WebsocketFlagMax
)

const (
	FlagRunning = iota
	FlagClosed

	FlagFrozen
	FlagActive

	FlagMax
)

type Conn struct {
	*websocket.Conn

	*Values

	sync.Mutex

	flags [WebsocketFlagMax]uint32

	id string

	raw net.Conn

	rawCommCnt uint64

	enc *JsonEncoder

	writeWait time.Duration

	pingPeriod time.Duration

	pongWait time.Duration

	isolatorTime time.Time

	ponglist  []interface{}
	closelist []interface{}
}

func newConn(c *websocket.Conn, v *Values) (*Conn, error) {
	if nil != c {
		if uuid, err := GenUUID(); nil == err {
			conn := &Conn{
				Conn: c,

				Values: v,

				id: uuid,

				raw: c.UnderlyingConn(),

				enc: NewJsonEncoder(),

				writeWait: 10 * time.Second,

				pingPeriod: 10 * time.Second,
			}

			atomic.StoreUint32(&conn.flags[WebsocketFlagClosed], FlagRunning)
			atomic.StoreUint32(&conn.flags[WebsocketFlagIdle], FlagFrozen)

			conn.pongWait = conn.pingPeriod*3 + time.Second

			return conn, nil
		} else {
			return nil, err
		}
	}
	return nil, syscall.EINVAL
}

func (this *Conn) Close() error {
	//
	if atomic.CompareAndSwapUint32(&this.flags[WebsocketFlagClosed], FlagRunning, FlagClosed) {
		// 回调
		for _, fn := range this.closelist {
			//
			switch _fn := fn.(type) {
			case func(string):
				_fn(this.id)
			case func(*Conn):
				_fn(this)
			case func(*Conn, string):
				_fn(this, this.id)
			}
		}
		// 关闭
		return this.Conn.Close()
	}
	return nil
}

func (this *Conn) ID() string {
	return this.id
}

func (this *Conn) StartIdlaCheck(timeout time.Duration, fns ...func(uint64, uint64)) {
	if FlagRunning == atomic.LoadUint32(&this.flags[WebsocketFlagClosed]) {
		if atomic.CompareAndSwapUint32(&this.flags[WebsocketFlagIdle], FlagFrozen, FlagActive) {
			CoroutineGo(func() {
				var now uint64
				//
				if time.Second > timeout {
					timeout = time.Second
				}
				//
				ticker := time.NewTicker(timeout)
				//
				defer ticker.Stop()
				//
				before := atomic.LoadUint64(&this.rawCommCnt)
				//
				for FlagRunning == atomic.LoadUint32(&this.flags[WebsocketFlagClosed]) {
					select {
					case <-ticker.C:
						now = atomic.LoadUint64(&this.rawCommCnt)
						//
						for _, fn := range fns {
							fn(before, now)
						}
						//
						if before == now {
							this.Close()
							return
						} else {
							//
							before = now
							//
							//logs.Info("OK: %d", atomic.LoadUint64(&this.rawCommCnt))
						}
					}
				}
			})
		}
	}
}

func (this *Conn) SetIsolatorTime(timeout time.Duration) {
	if time.Second < timeout {
		// 设置观察期
		this.isolatorTime = time.Now().Add(timeout)
	}
}

func (this *Conn) UnSetIsolatorTime() {
	// 复位观察期
	this.isolatorTime = time.Time{}
}

func (this *Conn) AddPongHandler(args ...interface{}) {
	//
	for _, item := range args {
		//
		switch item.(type) {
		case func(string):
		case func(*Conn):
		case func(*Conn, string):
		default:
			continue
		}
		//
		this.ponglist = append(this.ponglist, item)
	}
}

func (this *Conn) AddCloseHandler(args ...interface{}) {
	//
	for _, item := range args {
		//
		switch item.(type) {
		case func(string):
		case func(*Conn):
		case func(*Conn, string):
		default:
			continue
		}
		//
		this.closelist = append(this.closelist, item)
	}
}

func (this *Conn) Read(data []byte) (int, error) {
	// 通信次数+1
	atomic.AddUint64(&this.rawCommCnt, 1)
	// 底层通信
	return this.raw.Read(data)
}

func (this *Conn) Write(data []byte) (int, error) {
	// 通信次数+1
	atomic.AddUint64(&this.rawCommCnt, 1)
	// 底层通信
	return this.raw.Write(data)
}

func (this *Conn) SetDeadline(t time.Time) error {
	return this.raw.SetDeadline(t)
}

func (this *Conn) WriteMessage(messageType int, data []byte) error {
	if FlagRunning == atomic.LoadUint32(&this.flags[WebsocketFlagClosed]) {
		// 互斥
		this.Lock()
		defer this.Unlock()
		// 写超时
		this.SetWriteDeadline(time.Now().Add(this.writeWait))
		// 底层写
		return this.Conn.WriteMessage(messageType, data)
	} else {
		return ErrClosed
	}
}

func (this *Conn) WriteString(msg string) error {
	if "" != msg {
		if FlagRunning == atomic.LoadUint32(&this.flags[WebsocketFlagClosed]) {
			return this.WriteMessage(websocket.TextMessage, []byte(msg))
		} else {
			return ErrClosed
		}
	} else {
		return ErrEmptyData
	}
}

func (this *Conn) WriteJSON(v interface{}) (int, error) {
	if nil != v {
		if FlagRunning == atomic.LoadUint32(&this.flags[WebsocketFlagClosed]) {
			if nil != this.enc {
				this.enc.Lock()
				defer this.enc.Unlock()
				if err := this.enc.Encode(v); nil == err {
					if data := this.enc.Bytes(); nil != data {
						return this.enc.Len(), this.WriteMessage(websocket.TextMessage, data)
					} else {
						return 0, ErrEmptyData
					}
				} else {
					return 0, err
				}
			} else {
				if _data, err := json.Marshal(v); nil == err {
					return len(_data), this.WriteMessage(websocket.TextMessage, _data)
				} else {
					return 0, err
				}
			}
		} else {
			return 0, ErrClosed
		}
	} else {
		return 0, ErrEmptyData
	}
}

func (this *Conn) HandleConn(fn func(*Conn, string)) {
	if FlagRunning == atomic.LoadUint32(&this.flags[WebsocketFlagClosed]) {
		// 读缓冲
		var buf bytes.Buffer
		// 关闭检查channel
		exit := make(chan struct{})
		// 启动协程检测观察期
		if !this.isolatorTime.IsZero() {
			CoroutineGo(func() {
				ticker := time.NewTicker(time.Second)
				defer func() {
					// 关闭定时器
					ticker.Stop()
				}()
				for FlagRunning == atomic.LoadUint32(&this.flags[WebsocketFlagClosed]) {
					// 定时器
					<-ticker.C
					// 检查观察期
					if this.isolatorTime.IsZero() {
						// 退出协程
						return
					} else {
						// 是否超时
						if time.Now().After(this.isolatorTime) {
							// 警告信息
							logs.Warn("Isolator termination trigger closure")
							// 由协程关闭连接
							this.Close()
							// 退出协程
							return
						}
					}
				}
			})
		}
		// 启动协程发ping命令
		CoroutineGo(func() {
			ticker := time.NewTicker(this.pingPeriod)
			defer func() {
				// 关闭定时器
				ticker.Stop()
			}()
			for {
				select {
				case <-exit:
					// 由主协程关闭，直接退出
					return
				case <-ticker.C:
					if err := this.WriteMessage(websocket.PingMessage, nil); err != nil {
						logs.Error(this.id, ", WriteMessage(Ping): ", err)
						// 跳出循环
						break
					} /* else {
						logs.Info(this.id, ", WriteMessage(Ping): OK")
					}*/
				}
			}
			// 由协程关闭连接
			this.Close()
			// 等到channel释放
			<-exit
		})
		// 读超时
		this.SetReadDeadline(time.Now().Add(this.pongWait))
		// 注册pong回调
		this.SetPongHandler(func(string) error {
			//
			this.SetReadDeadline(time.Now().Add(this.pongWait))
			// 回调
			for _, fn := range this.ponglist {
				//
				switch _fn := fn.(type) {
				case func(string):
					_fn(this.id)
				case func(*Conn):
					_fn(this)
				case func(*Conn, string):
					_fn(this, this.id)
				}
			}
			//
			return nil
		})
		for FlagRunning == atomic.LoadUint32(&this.flags[WebsocketFlagClosed]) {
			if mt, r, err := this.NextReader(); nil == err {
				//if mt, msg, err := this.ReadMessage(); nil == err {
				if websocket.TextMessage == mt {
					// 清空缓冲
					buf.Reset()
					// 读数据到缓冲
					buf.ReadFrom(r)
					// 检查回调函数
					if nil != fn {
						fn(this, buf.String())
					} else {
						logs.Info("%s, ReadMessage(): mt: %d, %s\n", this.id, mt, buf.String())
					}
				} else {
					logs.Info("%s, ReadMessage(): mt: %d\n", this.id, mt)
				}
			} else {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
					logs.Error("error: %v", err)
				} else {
					logs.Error("error: %v", err)
				}
				break
			}
		}
		close(exit)
	}
}
