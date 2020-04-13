package websocket

import (
	"crypto/tls"
	"sync"
	"time"

	"github.com/elitah/utils/logs"

	"github.com/gorilla/websocket"
)

type Client struct {
	sync.RWMutex

	sync.Pool

	list map[string]*Conn

	dialer *websocket.Dialer
}

func NewClient() *Client {
	cli := &Client{
		list: make(map[string]*Conn),
	}
	cli.dialer = &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,

		ReadBufferSize:  1024,
		WriteBufferSize: 1024,

		WriteBufferPool: cli,
	}
	return cli
}

func (this *Client) Close() {
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

func (this *Client) SetTLSConfig(config *tls.Config) {
	this.dialer.TLSClientConfig = config
}

func (this *Client) Dial(urlStr string) (*Conn, error) {
	var errReturn error
	if conn, _, err := this.dialer.Dial(urlStr, nil); nil == err {
		if _conn, err := newConn(conn, &Values{}); nil == err {
			if this.listAdd(_conn.ID(), _conn) {
				_conn.AddCloseHandler(this.listDelete)
				return _conn, nil
			}
			// 关闭
			_conn.Close()
			// 错误
			errReturn = ErrListAdd
		} else {
			// 错误
			errReturn = err
		}
		// 关闭
		conn.Close()
	} else {
		errReturn = err
	}
	return nil, errReturn
}

func (this *Client) Len() int {
	this.RLock()
	defer this.RUnlock()

	return len(this.list)
}

func (this *Client) Lists() []string {
	var list []string

	this.Lock()

	for key, _ := range this.list {
		list = append(list, key)
	}

	this.Unlock()

	return list
}

func (this *Client) GetConnByID(id string) *Conn {
	this.RLock()
	defer this.RUnlock()

	if conn, ok := this.list[id]; ok {
		return conn
	}

	return nil
}

func (this *Client) listAdd(id string, conn *Conn) bool {
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

func (this *Client) listDelete(id string) {
	if "" != id {
		this.Lock()
		defer this.Unlock()

		// 移除列表
		delete(this.list, id)
	}
}
