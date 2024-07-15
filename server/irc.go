package main

import (
	"fmt"
	"strings"
)

type IrcState struct {
	server     string
	nick       string
	user       string
	host       string
	registered bool
}

func irc_newConnection(server string, host string) IrcState {
	return IrcState{
		server: server,
		host:   host,
	}
}

func irc_handleMessage(state *IrcState, message string) string {
	// tokens := irc_tokenize(message)
	command, params := irc_tokenize(message)

	handler, valid_command := ircCommands[command]
	if !valid_command {
		return fmt.Sprintf(":%v 421 %v :Unknown command\r\n", state.server, command)
	}

	return handler(state, params)
}

// Commands
// Dispatch table
var ircCommands = map[string](func(*IrcState, []string) string){
	"NICK": irc_nick,
	"USER": irc_user,
}

func irc_nick(state *IrcState, params []string) (response string) {
	if len(params) < 1 {
		return fmt.Sprintf(":%v 431 :No nickname given\r\n", state.server)
	}

	state.nick = params[0]

	if len(state.user) > 0 && !state.registered {
		state.registered = true
		return irc_rplWelcome(state.server, state.nick, state.user, state.host)
	}

	return ""
}
func irc_user(state *IrcState, params []string) (response string) {
	state.user = params[0]

	if len(state.nick) > 0 && !state.registered {
		state.registered = true
		return irc_rplWelcome(state.server, state.nick, state.user, state.host)
	}

	return ""
}

// utility functions
func irc_tokenize(message string) (command string, params []string) {
	message, _ = strings.CutSuffix(message, "\r\n")

	// params are a space delimited list of up to 14 parameters
	// followed by an optional trailer marked with a ":"
	message, trailing, found := strings.Cut(message, " :")
	tokens := strings.Split(message, " ")

	if found {
		tokens = append(tokens, trailing)
	}

	return tokens[0], tokens[1:]
}

func irc_rplWelcome(server string, nick string, user string, host string) string {
	const rplWelcomeFormat = ":%v 001 %v :Welcome to the Internet Relay Network %v!%v@%v\r\n"
	return fmt.Sprintf(rplWelcomeFormat, server, nick, nick, user, host)
}
