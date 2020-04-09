package websocket

import (
	"github.com/panjf2000/ants"
)

var (
	pool *ants.Pool = nil
)

func InitCoroutinePool(p *ants.Pool) {
	pool = p
}

func CoroutineGo(fn func()) {
	if nil != pool {
		pool.Submit(fn)
		return
	}
	go fn()
}
