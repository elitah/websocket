package websocket

import (
	"bytes"
	"encoding/json"
	"sync"
)

type JsonEncoder struct {
	sync.Mutex

	*bytes.Buffer
	*json.Encoder
}

func NewJsonEncoder() *JsonEncoder {
	buf := &bytes.Buffer{}
	if nil != buf {
		enc := json.NewEncoder(buf)
		if nil != enc {
			return &JsonEncoder{
				Buffer:  buf,
				Encoder: enc,
			}
		}
	}
	return nil
}

func (this *JsonEncoder) Encode(v interface{}) error {
	if nil != v {
		this.Reset()
		return this.Encoder.Encode(v)
	}
	return ErrBadInput
}

func (this *JsonEncoder) Bytes() []byte {
	if length := this.Len(); 0 < length {
		data := this.Buffer.Bytes()
		for ; 0 < length; length-- {
			if ']' == data[length-1] || '}' == data[length-1] {
				break
			}
		}
		if 1 < length {
			return data[:length]
		}
	}
	return nil
}

func (this *JsonEncoder) String() string {
	if data := this.Bytes(); nil != data {
		return string(data)
	}
	return ""
}
