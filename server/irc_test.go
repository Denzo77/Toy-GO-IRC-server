package main

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func makeTestConn() (client *bufio.ReadWriter, server net.Conn) {
	conn, server := net.Pipe()
	conn.SetDeadline(time.Now().Add(time.Second))
	client = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	return
}

func writeAndFlush(writer *bufio.ReadWriter, s string) {
	writer.WriteString(s)
	writer.Flush()
}

func discardResponse(reader *bufio.ReadWriter) {
	reader.ReadString('\n')
	reader.Reader.Discard(reader.Reader.Buffered())
}

func TestAssert(t *testing.T) {
	assert.Equal(t, 1+1, 2)
}

func TestUnknownCommandRespondsWithError(t *testing.T) {
	expected := ":bar.example.com 421 FOO :Unknown command\r\n"

	client, serverConn := makeTestConn()
	server := MakeServer("bar.example.com")
	newIrcConnection(server, serverConn)

	writeAndFlush(client, "FOO this fails\r\n")
	response, _ := client.ReadString('\n')

	assert.Equal(t, expected, response)
}

func TestRegisterUserRespondsWithRpl(t *testing.T) {
	tests := []struct {
		name   string
		first  string
		second string
	}{
		{"NICK then USER", "NICK nick\r\n", "USER user 0 * :Joe Bloggs\r\n"},
		{"USER then NICK", "USER user 0 * :Joe Bloggs\r\n", "NICK nick\r\n"},
	}

	// Response: RPL_WELCOME containing full client identifier
	expected := []string{
		":bar.example.com 001 nick :Welcome to the Internet Relay Network nick!user@pipe\r\n",
		":bar.example.com 002 nick :Your host is bar.example.com, running version 0.0\r\n",
		":bar.example.com 003 nick :This server was created 01/01/1970\r\n",
		":bar.example.com 004 nick :bar.example.com 0.0 0 0\r\n",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, serverConn := makeTestConn()
			server := MakeServer("bar.example.com")
			newIrcConnection(server, serverConn)

			writeAndFlush(client, tt.first)
			r, _ := client.ReadString('\n')
			assert.Equal(t, "\r\n", r)
			assert.Zero(t, client.Reader.Buffered())

			writeAndFlush(client, tt.second)

			response := []string{}
			for _ = range 4 {
				r, _ = client.ReadString('\n')
				response = append(response, r)
			}

			assert.Equal(t, expected, response)
			assert.Zero(t, client.Reader.Buffered())
		})
	}
}

func TestNickErrors(t *testing.T) {
	var tests = []struct {
		name     string
		input    string
		expected string
	}{
		{"ERR_NONICKNAMEGIVEN", "NICK\r\n", ":bar.example.com 431 :No nickname given\r\n"},
		// {"ERR_ERRONEUSNICKNAME", "NICK\r\n", ":bar.example.com 432 :<nick> : Erroneus nickname\r\n"},
		{"ERR_NICKNAMEINUSE", "NICK guest\r\n", ":bar.example.com 433 guest :Nickname is already in use\r\n"},
		// {"ERR_NICKCOLLISION", "NICK\r\n", ":bar.example.com 436 guest :Nickname collision KILL from <user>@<host>\r\n"},
		// {"ERR_UNAVAILABLERESOURCE", "NICK\r\n", ":bar.example.com 437 guest :Nick/channel is temporarily unavailable\r\n"},
		// {"ERR_RESTRICTED", "NICK\r\n", ":bar.example.com 484 :Your connection is restricted!\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			// register user
			client, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client, "NICK guest\r\n")
			discardResponse(client)
			writeAndFlush(client, "USER 0 * guest :Joe Blogs\r\n")
			discardResponse(client)

			// start test
			client2, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client2, "USER 0 * guest :Joe Blogs\r\n")
			discardResponse(client2)
			writeAndFlush(client2, tt.input)
			response, _ := client2.ReadString('\n')

			assert.Equal(t, tt.expected, response)
			assert.Zero(t, client.Reader.Buffered())
		})
	}
}

func TestUserErrors(t *testing.T) {
	var tests = []struct {
		name     string
		input    string
		expected string
	}{
		{"ERR_NEEDMOREPARAMS", "USER guest 0 *\r\n", ":bar.example.com 461 USER :Not enough parameters\r\n"},
		{"ERR_ALREADYREGISTERED", "USER guest 0 * :Joe Bloggs\r\n", ":bar.example.com 462 :Unauthorized command (already registered)\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			// register user
			client, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client, "NICK guest\r\n")
			discardResponse(client)
			writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
			discardResponse(client)

			writeAndFlush(client, tt.input)
			response, _ := client.ReadString('\n')

			assert.Equal(t, tt.expected, response)
			assert.Zero(t, client.Reader.Buffered())
		})
	}
}

func TestCommandsRejectedIfNotRegistered(t *testing.T) {
	var tests = []string{
		"QUIT\r\n",
		"PRIVMSG\r\n",
		"NOTICE\r\n",
		"PING\r\n",
		"PONG\r\n",
		"MOTD\r\n",
		"LUSERS\r\n",
		"WHOIS\r\n",
	}

	expected := ":bar.example.com 451 :You have not registered\r\n"

	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			client, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client, "NICK guest\r\n")
			discardResponse(client)

			writeAndFlush(client, command)
			response, _ := client.ReadString('\n')

			assert.Equal(t, expected, response)
			assert.Zero(t, client.Reader.Buffered())
		})
	}
}

func TestNickUpdatesNickName(t *testing.T) {
	server := MakeServer("bar.example.com")

	// register user
	client, serverConn := makeTestConn()
	newIrcConnection(server, serverConn)
	writeAndFlush(client, "NICK guest\r\n")
	discardResponse(client)
	writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
	discardResponse(client)

	writeAndFlush(client, "NICK notguest\r\n")
	response, _ := client.ReadString('\n')

	assert.Equal(t, ":guest NICK notguest\r\n", response)
	assert.Zero(t, client.Reader.Buffered())
}

func TestQuitEndsConnection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"QUIT with default message", "QUIT\r\n", ":bar.example.com ERROR :Closing Link: pipe Client Quit\r\n"},
		{"QUIT with custom message", "QUIT :Gone to have lunch\r\n", ":bar.example.com ERROR :Closing Link: pipe Gone to have lunch\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			// register user
			client, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client, "NICK guest\r\n")
			discardResponse(client)
			writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
			discardResponse(client)

			writeAndFlush(client, tt.input)
			response, _ := client.ReadString('\n')

			assert.Equal(t, tt.expected, response)
			assert.Zero(t, client.Reader.Buffered())

			// Test that user has been unregistered by checking if we can add them again.
			client2, serverConn2 := makeTestConn()
			newIrcConnection(server, serverConn2)
			writeAndFlush(client2, "NICK guest\r\n")
			response, _ = client2.ReadString('\n')

			assert.Equal(t, "\r\n", response)
			assert.Zero(t, client2.Reader.Buffered())

			//test that the connection has been shut
			_, err := serverConn.Read([]byte{})
			assert.NotNil(t, err)
		})

	}
}

func TestSending(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"PRIVMSG", "PRIVMSG receiver :This is a message\r\n", ":sender PRIVMSG receiver :This is a message\r\n"},
		{"NOTICE", "NOTICE receiver :This is a message\r\n", ":sender NOTICE receiver :This is a message\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			var newTestConn = func(nick string) (client *bufio.ReadWriter) {
				client, serverConn := makeTestConn()
				newIrcConnection(server, serverConn)
				writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
				discardResponse(client)
				writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
				discardResponse(client)

				return
			}

			sender := newTestConn("sender")
			receiver := newTestConn("receiver")

			writeAndFlush(sender, tt.input)
			discardResponse(sender)

			response, _ := receiver.ReadString('\n')
			assert.Equal(t, tt.expected, response)
		})
	}
}
func TestMessageSendingErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"PRIVMSG returns ERR_NOSUCHNICK", "PRIVMSG foo :Message\r\n", ":bar.example.com 401 sender foo :No such nick/channel\r\n"},
		{"PRIVMSG returns ERR_NORECIPIENT", "PRIVMSG \r\n", ":bar.example.com 411 sender :No recipient given (PRIVMSG)\r\n"},
		{"PRIVMSG returns ERR_NOTEXTTOSEND", "PRIVMSG reciever\r\n", ":bar.example.com 412 sender :No text to send\r\n"},
		{"NOTICE does not return ERR_NOSUCHNICK", "NOTICE foo :Message\r\n", "\r\n"},
		{"NOTICE does not return ERR_NORECIPIENT", "NOTICE \r\n", "\r\n"},
		{"NOTICE does not return ERR_NOTEXTTOSEND", "NOTICE reciever\r\n", "\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			var newTestConn = func(nick string) (client *bufio.ReadWriter) {
				client, serverConn := makeTestConn()
				newIrcConnection(server, serverConn)
				writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
				discardResponse(client)
				writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
				discardResponse(client)

				return
			}

			sender := newTestConn("sender")
			writeAndFlush(sender, tt.input)
			response, _ := sender.ReadString('\n')

			assert.Equal(t, tt.expected, response)
			assert.Zero(t, sender.Reader.Buffered())
		})
	}
}
