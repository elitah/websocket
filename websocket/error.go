package websocket

import (
	"errors"
	"net"
)

var (
	ErrClosed    = errors.New("conn closed")
	ErrListAdd   = errors.New("unable add to list")
	ErrBadInput  = errors.New("bad input")
	ErrEmptyData = errors.New("empty data")
)

func IsDialTimeout(err error) bool {
	if nil != err {
		if e, ok := err.(*net.OpError); ok {
			return e.Timeout()
		}
	}
	return false
}
