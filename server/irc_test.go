package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssert(t *testing.T) {
	assert.Equal(t, 1+1, 2)
}

func TestUnknownCommandRespondsWithError(t *testing.T) {
	expected := []string{":bar.example.com 421 FOO :Unknown command\r\n"}

	server := MakeServer("bar.example.com")
	state := newIrcConnection("foo.example.com")

	response, quit := handleIrcMessage(&server, &state, "FOO this fails\r\n")
	assert.Equal(t, expected, response)
	assert.False(t, quit)
}

func TestRegisterUserRespondsWithRplWelcome(t *testing.T) {
	// Expected messages
	// 1. Password (not implemented)
	// 2. Nickname message
	nick := "NICK nick\r\n"
	// 3. User message
	user := "USER user 0 * :Joe Bloggs\r\n"

	// Response: RPL_WELCOME containing full client identifier
	expected := []string{
		":bar.example.com 001 nick :Welcome to the Internet Relay Network nick!user@foo.example.com\r\n",
	}

	t.Run("NICK then USER", func(t *testing.T) {
		server := MakeServer("bar.example.com")
		state := newIrcConnection("foo.example.com")

		response, quit := handleIrcMessage(&server, &state, nick)
		assert.Equal(t, []string{}, response)
		assert.False(t, quit)

		response, quit = handleIrcMessage(&server, &state, user)
		assert.Equal(t, expected, response)
		assert.False(t, quit)
	})

	t.Run("USER then NICK", func(t *testing.T) {
		server := MakeServer("bar.example.com")
		state := newIrcConnection("foo.example.com")

		response, quit := handleIrcMessage(&server, &state, user)
		assert.Equal(t, []string{}, response)
		assert.False(t, quit)

		response, quit = handleIrcMessage(&server, &state, nick)
		assert.Equal(t, expected, response)
		assert.False(t, quit)
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
		handleIrcMessage(&server, &state, "NICK guest")
		return server, state
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, state := testServer()

			response, quit := handleIrcMessage(&server, &state, tt.input)
			assert.Equal(t, tt.expected, response)
			assert.False(t, quit)
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
		handleIrcMessage(&server, &state, "NICK guest")
		handleIrcMessage(&server, &state, "USER guest 0 * :Joe Bloggs")
		return server, state
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, state := testServer()

			response, quit := handleIrcMessage(&server, &state, tt.input)
			assert.Equal(t, tt.expected, response)
			assert.False(t, quit)
		})
	}
}

func TestQuitEndsConnection(t *testing.T) {
	testServer := func() (ServerInfo, connectionState) {
		server := MakeServer("bar.example.com")
		state := newIrcConnection("foo.example.com")
		handleIrcMessage(&server, &state, "NICK guest")
		handleIrcMessage(&server, &state, "USER guest 0 * :Joe Bloggs")

		return server, state
	}

	t.Run("QUIT with default message", func(t *testing.T) {
		server, state := testServer()

		response, quit := handleIrcMessage(&server, &state, "QUIT")
		assert.Equal(t, []string{":bar.example.com ERROR :Closing Link: foo.example.com Client Quit\r\n"}, response)
		assert.True(t, quit)

		// Test that user has been unregistered by checking if we can add them again.
		response, _ = handleIrcMessage(&server, &state, "NICK guest")
		assert.Equal(t, []string{}, response)
	})

	t.Run("QUIT with custom message", func(t *testing.T) {
		server, state := testServer()

		response, quit := handleIrcMessage(&server, &state, "QUIT :Gone to have lunch")
		assert.Equal(t, []string{":bar.example.com ERROR :Closing Link: foo.example.com Gone to have lunch\r\n"}, response)
		assert.True(t, quit)

		// Test that user has been unregistered by checking if we can add them again.
		response, _ = handleIrcMessage(&server, &state, "NICK guest")
		assert.Equal(t, []string{}, response)
	})
}
