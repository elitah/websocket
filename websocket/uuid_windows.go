// +build windows

package websocket

import (
	"crypto/rand"
	"fmt"
)

func GenUUID() (string, error) {
	//
	var buffer [16]byte
	//
	if _, err := rand.Read(buffer[:]); nil == err {
		//
		return fmt.Sprintf(
			"%x-%x-%x-%x-%x",
			buffer[0:4],
			buffer[4:6],
			buffer[6:8],
			buffer[8:10],
			buffer[10:16],
		), nil
	} else {
		//
		return "", err
	}
}
