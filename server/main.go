package main

import (
	"bufio"
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
		c, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}

		// launch coroutine for handling the new connection
		go handleConnection(c, &server)
		count++
	}
}

func handleConnection(conn net.Conn, server *ServerInfo) {
	fmt.Print(".")
	state := newIrcConnection(conn.RemoteAddr().String())

	for {
		// Should split on "\r\n"
		// See https://pkg.go.dev/bufio#Scanner & implementation of SplitLine
		// Could not get it to correctly handle EOF.
		netData, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}

		response, quit := handleIrcMessage(server, &state, netData)

		conn.Write([]byte(response))
		// fmt.Print("-> ", string(netData))
		// t := time.Now()
		// myTime := t.Format(time.RFC3339) + "\n"

		if quit {
			break
		}
	}

	conn.Close()
}
