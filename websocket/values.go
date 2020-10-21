package websocket

import (
	"fmt"
	"strings"
	"sync"
)

type Values struct {
	sync.Map
}

func (this *Values) KVSet(key string, value interface{}) {
	if "" != key && nil != value {
		this.Store(key, value)
	}
}

func (this *Values) KVUnSet(key string) {
	this.Map.Delete(key)
}

func (this *Values) KVClear() {
	this.Range(func(key, value interface{}) bool {
		this.Map.Delete(key)
		return true
	})
}

func (this *Values) KVGet(key string) interface{} {
	if v, ok := this.Load(key); ok {
		return v
	}
	return nil
}

func (this *Values) KVGetString(key string, def ...string) string {
	if v := this.KVGet(key); nil != v {
		if r, ok := v.(string); ok {
			return r
		}
	}

	if 0 < len(def) {
		return def[0]
	}

	return ""
}

func (this *Values) KVGetBoolean(key string, def ...bool) bool {
	if v := this.KVGet(key); nil != v {
		if r, ok := v.(bool); ok {
			return r
		}
	}

	if 0 < len(def) {
		return def[0]
	}

	return false
}

func (this *Values) KVGetByte(key string, def ...byte) byte {
	if v := this.KVGet(key); nil != v {
		if r, ok := v.(byte); ok {
			return r
		}
	}

	if 0 < len(def) {
		return def[0]
	}

	return 0
}

func (this *Values) KVGetInt(key string, def ...int) int {
	if v := this.KVGet(key); nil != v {
		if r, ok := v.(int); ok {
			return r
		}
	}

	if 0 < len(def) {
		return def[0]
	}

	return 0
}

func (this *Values) KVGetInt64(key string, def ...int64) int64 {
	if v := this.KVGet(key); nil != v {
		if r, ok := v.(int64); ok {
			return r
		}
	}

	if 0 < len(def) {
		return def[0]
	}

	return 0
}

func (this *Values) String() string {
	//
	var b strings.Builder
	//
	this.Map.Range(func(key, value interface{}) bool {
		//
		fmt.Fprintf(&b, "%v: %v\n", key, value)
		//
		return true
	})
	//
	return b.String()
}
