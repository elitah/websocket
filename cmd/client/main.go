package main

import (
	"fmt"
	"time"

	"github.com/elitah/websocket/websocket"
)

func main() {
	ws := websocket.NewClient()

	if conn, err := ws.Dial("ws://127.0.0.1:8788/ws"); nil == err {
		go func() {
			for {
				conn.WriteString("hello")

				time.Sleep(3 * time.Second)
			}
		}()

		conn.HandleConn(func(conn *websocket.Conn, msg string) {
			fmt.Println(msg)
		})
	} else {
		fmt.Println(err)
	}
}
