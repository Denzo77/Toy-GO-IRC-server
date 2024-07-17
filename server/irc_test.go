package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Reads all the elements from a channel and returns the results as a string.
func drainChannel(channel chan string) []string {
	var response []string
	for r := range channel {
		response = append(response, r)
	}

	return response
}

func TestAssert(t *testing.T) {
	assert.Equal(t, 1+1, 2)
}

func TestUnknownCommandRespondsWithError(t *testing.T) {
	expected := ":bar.example.com 421 FOO :Unknown command\r\n"

	server := MakeServer("bar.example.com")
	state := newIrcConnection("foo.example.com")

	responseChan, quitChan := handleIrcMessage(&server, &state, "FOO this fails\r\n")
	assert.Equal(t, expected, <-responseChan)
	assert.Empty(t, quitChan)
}

func TestRegisterUserRespondsWithRpl(t *testing.T) {
	// Expected messages
	// 1. Password (not implemented)
	// 2. Nickname message
	nick := "NICK nick\r\n"
	// 3. User message
	user := "USER user 0 * :Joe Bloggs\r\n"

	// Response: RPL_WELCOME containing full client identifier
	expected := []string{
		":bar.example.com 001 nick :Welcome to the Internet Relay Network nick!user@foo.example.com\r\n",
		":bar.example.com 002 nick :Your host is bar.example.com, running version 0.0\r\n",
		":bar.example.com 003 nick :This server was created 01/01/1970\r\n",
		":bar.example.com 004 nick :bar.example.com 0.0 0 0\r\n",
	}

	t.Run("NICK then USER", func(t *testing.T) {
		server := MakeServer("bar.example.com")
		state := newIrcConnection("foo.example.com")

		responseChan, quitChan := handleIrcMessage(&server, &state, nick)
		response := drainChannel(responseChan)
		assert.Nil(t, response)
		assert.Empty(t, quitChan)

		responseChan, quitChan = handleIrcMessage(&server, &state, user)
		response = drainChannel(responseChan)
		assert.Equal(t, expected, response)
		assert.Empty(t, quitChan)
	})

	t.Run("USER then NICK", func(t *testing.T) {
		server := MakeServer("bar.example.com")
		state := newIrcConnection("foo.example.com")

		responseChan, quitChan := handleIrcMessage(&server, &state, user)
		response := drainChannel(responseChan)
		assert.Nil(t, response)
		assert.Empty(t, quitChan)

		responseChan, quitChan = handleIrcMessage(&server, &state, nick)
		response = drainChannel(responseChan)
		assert.Equal(t, expected, response)
		assert.Empty(t, quitChan)
	})
}

func TestNickErrors(t *testing.T) {
	var tests = []struct {
		name     string
		input    string
		expected []string
	}{
		{"ERR_NONICKNAMEGIVEN", "NICK", []string{":bar.example.com 431 :No nickname given\r\n"}},
		// {"ERR_ERRONEUSNICKNAME", "NICK", ":bar.example.com 432 :<nick> : Erroneus nickname\r\n"},
		{"ERR_NICKNAMEINUSE", "NICK guest", []string{":bar.example.com 433 guest :Nickname is already in use\r\n"}},
		// {"ERR_NICKCOLLISION", "NICK", ":bar.example.com 436 guest :Nickname collision KILL from <user>@<host>\r\n"},
		// {"ERR_UNAVAILABLERESOURCE", "NICK", ":bar.example.com 437 guest :Nick/channel is temporarily unavailable\r\n"},
		// {"ERR_RESTRICTED", "NICK", ":bar.example.com 484 :Your connection is restricted!\r\n"},
	}

	testServer := func() (ServerInfo, connectionState) {
		server := MakeServer("bar.example.com")
		state := newIrcConnection("foo.example.com")
		responseChan, _ := handleIrcMessage(&server, &state, "NICK guest")
		<-responseChan
		return server, state
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, state := testServer()

			responseChan, quitChan := handleIrcMessage(&server, &state, tt.input)
			response := drainChannel(responseChan)
			assert.Equal(t, tt.expected, response)
			assert.Empty(t, quitChan)
		})
	}
}

func TestUserErrors(t *testing.T) {
	var tests = []struct {
		name     string
		input    string
		expected []string
	}{
		{"ERR_NEEDMOREPARAMS", "USER guest 0 *", []string{":bar.example.com 461 USER :Not enough parameters\r\n"}},
		{"ERR_ALREADYREGISTERED", "USER guest 0 * :Joe Bloggs", []string{":bar.example.com 462 :Unauthorized command (already registered)\r\n"}},
	}

	testServer := func() (ServerInfo, connectionState) {
		server := MakeServer("bar.example.com")
		state := newIrcConnection("foo.example.com")
		responseChan, _ := handleIrcMessage(&server, &state, "NICK guest")
		<-responseChan
		responseChan, _ = handleIrcMessage(&server, &state, "USER guest 0 * :Joe Bloggs")
		<-responseChan
		return server, state
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, state := testServer()

			responseChan, quitChan := handleIrcMessage(&server, &state, tt.input)

			assert.Equal(t, tt.expected, drainChannel(responseChan))
			assert.Empty(t, quitChan)
		})
	}
}

func TestCommandsRejectedIfNotRegistered(t *testing.T) {
	var tests = []string{
		"QUIT",
		"PRIVMSG",
		"NOTICE",
		"PING",
		"PONG",
		"MOTD",
		"LUSERS",
		"WHOIS",
	}

	expected := []string{":bar.example.com 451 :You have not registered\r\n"}

	for _, command := range tests {
		t.Run(command, func(t *testing.T) {
			server := MakeServer("bar.example.com")
			state := newIrcConnection("foo.example.com")
			responseChan, quitChan := handleIrcMessage(&server, &state, command)
			assert.Equal(t, expected, drainChannel(responseChan))
			assert.Empty(t, quitChan)
		})
	}
}

func TestNickUpdatesNickName(t *testing.T) {
	// Setup
	server := MakeServer("bar.example.com")
	state := newIrcConnection("foo.example.com")
	responseChan, _ := handleIrcMessage(&server, &state, "NICK guest")
	<-responseChan
	responseChan, _ = handleIrcMessage(&server, &state, "USER guest 0 * :Joe Bloggs")
	<-responseChan

	// Test
	responseChan, quitChan := handleIrcMessage(&server, &state, "NICK notguest")
	assert.Equal(t, []string{":guest NICK notguest\r\n"}, drainChannel(responseChan))
	assert.Empty(t, quitChan)
}

func TestQuitEndsConnection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"QUIT with default message", "QUIT", []string{":bar.example.com ERROR :Closing Link: foo.example.com Client Quit\r\n"}},
		{"QUIT with custom message", "QUIT :Gone to have lunch", []string{":bar.example.com ERROR :Closing Link: foo.example.com Gone to have lunch\r\n"}},
	}

	testServer := func() (ServerInfo, connectionState) {
		server := MakeServer("bar.example.com")
		state := newIrcConnection("foo.example.com")
		responseChan, _ := handleIrcMessage(&server, &state, "NICK guest")
		<-responseChan
		responseChan, _ = handleIrcMessage(&server, &state, "USER guest 0 * :Joe Bloggs")
		<-responseChan

		return server, state
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, state := testServer()

			responseChan, quitChan := handleIrcMessage(&server, &state, tt.input)
			assert.Equal(t, tt.expected, drainChannel(responseChan))
			assert.True(t, <-quitChan)

			// Test that user has been unregistered by checking if we can add them again.
			state = newIrcConnection("foo.example.com")
			responseChan, _ = handleIrcMessage(&server, &state, "NICK guest")
			assert.Nil(t, drainChannel(responseChan))
		})

	}
}

// func TestPrivmsg(t *testing.T) {
// 	server := MakeServer("bar.example.com")

// 	var newTestConn = func(nick string) (conn connectionState) {
// 		conn = newIrcConnection("foo.example.com")
// 		responseChan, _ := handleIrcMessage(&server, &conn, fmt.Sprintf("NICK %v", nick))
// 		<-responseChan
// 		responseChan, _ = handleIrcMessage(&server, &conn, fmt.Sprintf("USER %v 0 * :%v", nick, nick))
// 		<-responseChan
// 		return
// 	}

// 	sender := newTestConn("sender")
// 	receiver := newTestConn("receiver")

// 	responseChan, quitChan := handleIrcMessage(&server, &sender, "PRIVMSG receiver :This is a message")
// 	assert.Equal(t, []string{}, drainChannel(responseChan))
// 	assert.Empty(t, quitChan)

// 	assert.Equal(t, []string{}, drainChannel(responseChan))
// 	assert.Empty(t, quitChan)
// }
