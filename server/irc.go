package main

import (
	"fmt"
	"strings"
)

type connectionState struct {
	nick       string
	user       string
	host       string
	registered bool
}

type serverInfo struct {
	name string
}

func newIrcConnection(host string) connectionState {
	return connectionState{
		host: host,
	}
}

func handleIrcMessage(server *serverInfo, state *connectionState, message string) string {
	// tokens := irc_tokenize(message)
	command, params := tokenize(message)

	handler, valid_command := ircCommands[command]
	if !valid_command {
		return fmt.Sprintf(":%v 421 %v :Unknown command\r\n", server.name, command)
	}

	return handler(server, state, params)
}

// Commands
// Dispatch table
var ircCommands = map[string](func(*serverInfo, *connectionState, []string) string){
	"NICK": handleNick,
	"USER": handleUser,
}

func handleNick(server *serverInfo, state *connectionState, params []string) (response string) {
	if len(params) < 1 {
		return fmt.Sprintf(":%v 431 :No nickname given\r\n", server.name)
	}

	state.nick = params[0]

	if len(state.user) > 0 && !state.registered {
		state.registered = true
		return rplWelcome(server.name, state.nick, state.user, state.host)
	}

	return ""
}
func handleUser(server *serverInfo, state *connectionState, params []string) (response string) {
	state.user = params[0]

	if len(state.nick) > 0 && !state.registered {
		state.registered = true
		return rplWelcome(server.name, state.nick, state.user, state.host)
	}

	return ""
}

// utility functions
func tokenize(message string) (command string, params []string) {
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

func rplWelcome(server string, nick string, user string, host string) string {
	const rplWelcomeFormat = ":%v 001 %v :Welcome to the Internet Relay Network %v!%v@%v\r\n"
	return fmt.Sprintf(rplWelcomeFormat, server, nick, nick, user, host)
}
