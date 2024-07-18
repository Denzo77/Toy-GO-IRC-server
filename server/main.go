package main

import (
	"fmt"
	"net"
	"os"
	// "strconv"
	// "strings"
	// "time"
)

var count = 0

func main() {
	arguments := os.Args
	if len(arguments) == 1 {
		fmt.Println("Please provide port number")
		return
	}

	PORT := ":" + arguments[1]
	l, err := net.Listen("tcp", PORT)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer l.Close()

	server := MakeServer(l.Addr().String())

	for {
		// blocks until a new connection comes in
		conn, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}

		// launch coroutine for handling the new connection
		newIrcConnection(server, conn)
		count++
	}
}
