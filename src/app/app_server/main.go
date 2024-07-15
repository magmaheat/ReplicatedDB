package main

import (
	"Go_Team01/src/server"
)

func main() {
	srv := server.NewServer("127.0.0.1:8765", 3)
	srv.Run()
}
