package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssert(t *testing.T) {
	assert.Equal(t, 1+1, 2)
}

func TestUnknownCommandRespondsWithError(t *testing.T) {
	expected := ":bar.example.com 421 FOO :Unknown command\r\n"

	server := serverInfo{"bar.example.com"}
	state := newIrcConnection("foo.example.com")
	assert.Equal(t, expected, handleIrcMessage(&server, &state, "FOO this fails\r\n"))
}

func TestRegisterUserRespondsWithRplWelcome(t *testing.T) {
	// Expected messages
	// 1. Password (not implemented)
	// 2. Nickname message
	nick := "NICK nick\r\n"
	// 3. User message
	user := "USER user 0 * :Joe Bloggs\r\n"

	// Response: RPL_WELCOME containing full client identifier
	expected := ":bar.example.com 001 nick :Welcome to the Internet Relay Network nick!user@foo.example.com\r\n"

	t.Run("NICK then USER", func(t *testing.T) {
		server := serverInfo{"bar.example.com"}
		state := newIrcConnection("foo.example.com")
		assert.Equal(t, "", handleIrcMessage(&server, &state, nick))
		assert.Equal(t, expected, handleIrcMessage(&server, &state, user))
	})

	t.Run("USER then NICK", func(t *testing.T) {
		server := serverInfo{"bar.example.com"}
		state := newIrcConnection("foo.example.com")
		assert.Equal(t, "", handleIrcMessage(&server, &state, user))
		assert.Equal(t, expected, handleIrcMessage(&server, &state, nick))
	})
}

func TestNickErrors(t *testing.T) {
	var tests = []struct {
		name     string
		input    string
		expected string
	}{
		{"ERR_NONICKNAMEGIVEN", "NICK", ":bar.example.com 431 :No nickname given\r\n"},
		// {"ERR_ERRONEUSNICKNAME", "NICK", ":bar.example.com 432 :<nick> : Erroneus nickname\r\n"},
		{"ERR_NICKNAMEINUSE", "NICK guest", ":bar.example.com 433 guest :Nickname is already in use\r\n"},
		// {"ERR_NICKCOLLISION", "NICK", ":bar.example.com 436 guest :Nickname collision KILL from <user>@<host>\r\n"},
		// {"ERR_UNAVAILABLERESOURCE", "NICK", ":bar.example.com 437 guest :Nick/channel is temporarily unavailable\r\n"},
		// {"ERR_RESTRICTED", "NICK", ":bar.example.com 484 :Your connection is restricted!\r\n"},
	}

	testServer := func() (serverInfo, connectionState) {
		server := serverInfo{"bar.example.com"}
		state := newIrcConnection("foo.example.com")
		return server, state
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, state := testServer()
			assert.Equal(t, tt.expected, handleIrcMessage(&server, &state, tt.input))
		})
	}
}
