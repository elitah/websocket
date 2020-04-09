package main

import (
	"fmt"
	"net/http"

	"github.com/elitah/websocket/websocket"
)

func main() {
	ws := websocket.NewServer(func(conn *websocket.Conn, msg string) {
		conn.WriteString(msg)
	})

	http.ListenAndServe(":8788", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, "hello world")
			return
		case "/ws":
			ws.Upgrade(w, r)
			return
		}
		http.NotFound(w, r)
	}))
}
