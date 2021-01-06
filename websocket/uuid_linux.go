// +build linux

package websocket

import (
	"io"
	"io/ioutil"
)

func GenUUID() (string, error) {
	if data, err := ioutil.ReadFile("/proc/sys/kernel/random/uuid"); nil == err {
		if 36 <= len(data) {
			return string(data[:36]), nil
		} else {
			return "", io.ErrUnexpectedEOF
		}
	} else {
		return "", err
	}
}
