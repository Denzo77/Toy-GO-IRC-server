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

	state := irc_newConnection("bar.example.com", "foo.example.com")
	assert.Equal(t, expected, irc_handleMessage(&state, "FOO this fails\r\n"))
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
		state := irc_newConnection("bar.example.com", "foo.example.com")
		assert.Equal(t, "", irc_handleMessage(&state, nick))
		assert.Equal(t, expected, irc_handleMessage(&state, user))
	})

	t.Run("USER then NICK", func(t *testing.T) {
		state := irc_newConnection("bar.example.com", "foo.example.com")
		assert.Equal(t, "", irc_handleMessage(&state, user))
		assert.Equal(t, expected, irc_handleMessage(&state, nick))
	})
}
