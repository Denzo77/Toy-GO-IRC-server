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

func discardResponse(reader *bufio.ReadWriter, min_number uint) {
	for _ = range min_number {
		reader.ReadString('\n')
	}

	reader.Reader.Discard(reader.Reader.Buffered())
}

func TestAssert(t *testing.T) {
	assert.Equal(t, 1+1, 2)
}

func TestUnknownCommandRespondsWithError(t *testing.T) {
	expected := ":bar.example.com 421 * FOO :Unknown command\r\n"

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
			for _ = range len(expected) {
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
		{"ERR_NONICKNAMEGIVEN", "NICK\r\n", ":bar.example.com 431 * :No nickname given\r\n"},
		// {"ERR_ERRONEUSNICKNAME", "NICK\r\n", ":bar.example.com 432 :<nick> : Erroneus nickname\r\n"},
		{"ERR_NICKNAMEINUSE", "NICK guest\r\n", ":bar.example.com 433 * guest :Nickname is already in use\r\n"},
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
			discardResponse(client, 1)
			writeAndFlush(client, "USER 0 * guest :Joe Blogs\r\n")
			discardResponse(client, 4)

			// start test
			client2, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client2, "USER 0 * guest :Joe Blogs\r\n")
			discardResponse(client2, 1)
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
		{"ERR_NEEDMOREPARAMS", "USER guest 0 *\r\n", ":bar.example.com 461 guest USER :Not enough parameters\r\n"},
		{"ERR_ALREADYREGISTERED", "USER guest 0 * :Joe Bloggs\r\n", ":bar.example.com 462 guest :Unauthorized command (already registered)\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			// register user
			client, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client, "NICK guest\r\n")
			discardResponse(client, 1)
			writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
			discardResponse(client, 4)

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
		"JOIN\r\n",
		"PART\r\n",
		"TOPIC\r\n",
		"AWAY\r\n",
		"NAMES\r\n",
		"LIST\r\n",
		"WHO\r\n",
	}

	expected := ":bar.example.com 451 guest :You have not registered\r\n"

	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			client, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client, "NICK guest\r\n")
			discardResponse(client, 1)

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
	discardResponse(client, 1)
	writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
	discardResponse(client, 4)

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
		// FIXME: Should these return the origin field (:bar.example.com)
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
			discardResponse(client, 1)
			writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
			discardResponse(client, 4)

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

func TestSendingDMs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"PRIVMSG", "PRIVMSG receiver :This is a message\r\n", ":sender!sender@pipe PRIVMSG receiver :This is a message\r\n"},
		{"NOTICE", "NOTICE receiver :This is a message\r\n", ":sender!sender@pipe NOTICE receiver :This is a message\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			var newTestConn = func(nick string) (client *bufio.ReadWriter) {
				client, serverConn := makeTestConn()
				newIrcConnection(server, serverConn)
				writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
				discardResponse(client, 1)
				writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
				discardResponse(client, 4)

				return
			}

			sender := newTestConn("sender")
			receiver := newTestConn("receiver")

			writeAndFlush(sender, tt.input)
			discardResponse(sender, 1)

			response, _ := receiver.ReadString('\n')
			assert.Equal(t, tt.expected, response)
		})
	}
}
func TestSendingToChannels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"PRIVMSG", "PRIVMSG #test :This is a message\r\n", ":sender!sender@pipe PRIVMSG #test :This is a message\r\n"},
		{"NOTICE", "NOTICE #test :This is a message\r\n", ":sender!sender@pipe NOTICE #test :This is a message\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			var newTestConn = func(nick string) (client *bufio.ReadWriter) {
				client, serverConn := makeTestConn()
				newIrcConnection(server, serverConn)
				writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
				discardResponse(client, 1)
				writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
				discardResponse(client, 4)
				writeAndFlush(client, "JOIN #test\r\n")
				discardResponse(client, 4)

				return
			}

			sender := newTestConn("sender")
			receiver1 := newTestConn("receiver1")
			receiver2 := newTestConn("receiver2")

			// Discard channel join messages
			discardResponse(sender, 2)
			discardResponse(receiver1, 1)

			writeAndFlush(sender, tt.input)
			discardResponse(sender, 1)

			response, _ := receiver1.ReadString('\n')
			assert.Equal(t, tt.expected, response)

			response, _ = receiver2.ReadString('\n')
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
		{"PRIVMSG returns ERR_NOSUCHNICK when unknown nick", "PRIVMSG foo :Message\r\n", ":bar.example.com 401 sender foo :No such nick/channel\r\n"},
		{"PRIVMSG returns ERR_NOSUCHNICK when unknown channel", "PRIVMSG #test :Message\r\n", ":bar.example.com 401 sender #test :No such nick/channel\r\n"},
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
				discardResponse(client, 1)
				writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
				discardResponse(client, 4)

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

func TestPingingServer(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"replies with PONG", "PING bar.example.com\r\n", ":bar.example.com PONG bar.example.com bar.example.com\r\n"},
		{"ERR_NEEDMOREPARAMS", "PING \r\n", ":bar.example.com 461 guest PING :Not enough parameters\r\n"},
		// TODO: Not sure what conditions should trigger this error
		// {"ERR_NOORIGIN", "\r\n", ":bar.example.com 409 guest :No origin specified\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			// register user
			client, serverConn := makeTestConn()
			newIrcConnection(server, serverConn)
			writeAndFlush(client, "NICK guest\r\n")
			discardResponse(client, 1)
			writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
			discardResponse(client, 4)

			writeAndFlush(client, tt.input)
			response, _ := client.ReadString('\n')

			assert.Equal(t, tt.expected, response)
			assert.Zero(t, client.Reader.Buffered())
		})
	}
}

func TestPongingServerDoesNotRespond(t *testing.T) {
	server := MakeServer("bar.example.com")

	// register user
	client, serverConn := makeTestConn()
	newIrcConnection(server, serverConn)
	writeAndFlush(client, "NICK guest\r\n")
	discardResponse(client, 1)
	writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
	discardResponse(client, 4)

	writeAndFlush(client, "PONG :bar.example.com\r\n")
	response, _ := client.ReadString('\n')

	assert.Equal(t, "\r\n", response)
	assert.Zero(t, client.Reader.Buffered())
}

func TestMotdErrors(t *testing.T) {
	server := MakeServer("bar.example.com")

	// register user
	client, serverConn := makeTestConn()
	newIrcConnection(server, serverConn)
	writeAndFlush(client, "NICK guest\r\n")
	discardResponse(client, 1)
	writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
	discardResponse(client, 4)

	writeAndFlush(client, "MOTD\r\n")
	response, _ := client.ReadString('\n')

	assert.Equal(t, ":bar.example.com 422 guest :MOTD not implemented\r\n", response)
	assert.Zero(t, client.Reader.Buffered())
}

func TestLusers(t *testing.T) {
	input := "LUSERS\r\n"
	expected := []string{
		":bar.example.com 251 sender :There are 2 users and 0 invisible on 0 servers\r\n",
		":bar.example.com 252 sender 0 :operator(s) online\r\n",
		":bar.example.com 253 sender 1 :unknown connection(s)\r\n",
		":bar.example.com 254 sender 0 :channels formed\r\n",
		":bar.example.com 255 sender :I have 3 clients and 0 servers\r\n",
	}

	server := MakeServer("bar.example.com")

	var newTestConn = func(nick string) (client *bufio.ReadWriter) {
		client, serverConn := makeTestConn()
		newIrcConnection(server, serverConn)
		writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
		discardResponse(client, 1)
		writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
		discardResponse(client, 4)

		return
	}

	sender := newTestConn("sender")
	_ = newTestConn("guest1")

	// Incomplete registration
	guest2, serverConn := makeTestConn()
	newIrcConnection(server, serverConn)
	writeAndFlush(guest2, "NICK guest2\r\n")
	discardResponse(guest2, 1)

	writeAndFlush(sender, input)
	response := []string{}
	for _ = range len(expected) {

		r, _ := sender.ReadString('\n')
		response = append(response, r)
	}

	assert.Equal(t, expected, response)
	assert.Zero(t, sender.Reader.Buffered())
}

func TestWhois(t *testing.T) {
	input := "WHOIS guest\r\n"
	expected := []string{
		":bar.example.com 311 sender guest guest pipe :Joe Bloggs\r\n",
		":bar.example.com 312 sender guest bar.example.com :Toy server\r\n",
		":bar.example.com 318 sender guest :End of /WHOIS list\r\n",
	}

	server := MakeServer("bar.example.com")

	var newTestConn = func(nick string) (client *bufio.ReadWriter) {
		client, serverConn := makeTestConn()
		newIrcConnection(server, serverConn)
		writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
		discardResponse(client, 1)
		writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
		discardResponse(client, 4)

		return
	}

	sender := newTestConn("sender")
	_ = newTestConn("guest")

	writeAndFlush(sender, input)
	response := []string{}
	for _ = range len(expected) {
		r, _ := sender.ReadString('\n')
		response = append(response, r)
	}

	assert.Equal(t, expected, response)
	assert.Zero(t, sender.Reader.Buffered())
}

func TestWhoisErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"No parameters does not respond", "WHOIS\r\n", "\r\n"},
		{"ERR_NOSUCHNICK", "WHOIS foo\r\n", ":bar.example.com 401 sender foo :No such nick/channel\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			var newTestConn = func(nick string) (client *bufio.ReadWriter) {
				client, serverConn := makeTestConn()
				newIrcConnection(server, serverConn)
				writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
				discardResponse(client, 1)
				writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
				discardResponse(client, 4)

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

func TestJoin(t *testing.T) {
	input := "JOIN #test\r\n"

	server := MakeServer("bar.example.com")

	var newTestConn = func(nick string) (client *bufio.ReadWriter) {
		client, serverConn := makeTestConn()
		newIrcConnection(server, serverConn)
		writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
		discardResponse(client, 1)
		writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
		discardResponse(client, 4)

		return
	}

	// Channel creation
	expected := []string{
		":creator!creator@pipe JOIN #test\r\n",
		":bar.example.com 332 creator #test :Test\r\n",
		// ":bar.example.com 333 creator #test creator <timestamp>\r\n", // TODO: RPL_TOPICWHOTIME
		":bar.example.com 353 creator = #test :+creator\r\n",
		":bar.example.com 366 creator #test :End of /NAMES list\r\n",
	}
	creator := newTestConn("creator")
	writeAndFlush(creator, input)
	response := []string{}
	for _ = range len(expected) {
		r, _ := creator.ReadString('\n')
		response = append(response, r)
	}
	assert.Equal(t, expected, response)
	assert.Zero(t, creator.Reader.Buffered())

	// Another user joins
	expected = []string{
		":guest!guest@pipe JOIN #test\r\n",
		":bar.example.com 332 guest #test :Test\r\n",
		// ":bar.example.com 333 creator #test creator <timestamp>\r\n", // TODO: RPL_TOPICWHOTIME
		":bar.example.com 353 guest = #test :+creator +guest\r\n",
		":bar.example.com 366 guest #test :End of /NAMES list\r\n",
	}
	guest := newTestConn("guest")
	writeAndFlush(guest, input)
	response = []string{}
	for _ = range len(expected) {
		r, _ := guest.ReadString('\n')
		response = append(response, r)
	}
	assert.Equal(t, expected, response, "Note may fail spuriously due to '+creator' and '+guest' being swapped")
	assert.Zero(t, guest.Reader.Buffered())

	// Check message to creator
	r, _ := creator.ReadString('\n')
	assert.Equal(t, ":guest!guest@pipe JOIN #test\r\n", r)
	assert.Zero(t, creator.Reader.Buffered())
}

func TestJoinErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ERR_NEEDMOREPARAMS", "JOIN \r\n", ":bar.example.com 461 guest JOIN :Not enough parameters\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			var newTestConn = func(nick string) (client *bufio.ReadWriter) {
				client, serverConn := makeTestConn()
				newIrcConnection(server, serverConn)
				writeAndFlush(client, "NICK guest\r\n")
				discardResponse(client, 1)
				writeAndFlush(client, "USER guest 0 * :Joe Bloggs\r\n")
				discardResponse(client, 4)

				return
			}

			sender := newTestConn("guest")

			writeAndFlush(sender, tt.input)
			response, _ := sender.ReadString('\n')

			assert.Equal(t, tt.expected, response)
			assert.Zero(t, sender.Reader.Buffered())
		})
	}
}

func TestPart(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"with no message", "PART #test\r\n", ":guest!guest@pipe PART #test\r\n"},
		{"with message", "PART #test :Goodbye\r\n", ":guest!guest@pipe PART #test :Goodbye\r\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := MakeServer("bar.example.com")

			var newTestConn = func(nick string) (client *bufio.ReadWriter) {
				client, serverConn := makeTestConn()
				newIrcConnection(server, serverConn)
				writeAndFlush(client, fmt.Sprintf("NICK %v\r\n", nick))
				discardResponse(client, 1)
				writeAndFlush(client, fmt.Sprintf("USER %v 0 * :Joe Bloggs\r\n", nick))
				discardResponse(client, 4)

				return
			}

			// Setup
			creator := newTestConn("creator")
			writeAndFlush(creator, "JOIN #test\r\n")
			discardResponse(creator, 4)

			// Another user joins
			guest := newTestConn("guest")
			writeAndFlush(guest, "JOIN #test\r\n")
			discardResponse(guest, 4)
			discardResponse(creator, 1)

			// User leaves
			writeAndFlush(guest, tt.input)
			r, _ := creator.ReadString('\n')
			assert.Equal(t, tt.expected, r)
			assert.Zero(t, creator.Reader.Buffered())

			r, _ = guest.ReadString('\n')
			assert.Equal(t, tt.expected, r)
			assert.Zero(t, guest.Reader.Buffered())
		})
	}
}
