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

func newIrcConnection(host string) connectionState {
	return connectionState{
		host: host,
	}
}

func handleIrcMessage(server *ServerInfo, state *connectionState, message string) (response []string, quit bool) {
	// tokens := irc_tokenize(message)
	command, params := tokenize(message)

	// Slightly hacky special case to avoid editing all command handlers
	// TODO: May need to change anyway in the future.
	if command == "QUIT" {
		return handleQuit(server, state, params), true
	}

	handler, valid_command := ircCommands[command]
	if !valid_command {
		return []string{fmt.Sprintf(":%v 421 %v :Unknown command\r\n", server.name, command)}, false
	}

	return handler(server, state, params), false
}

// Commands
// Dispatch table
var ircCommands = map[string](func(*ServerInfo, *connectionState, []string) []string){
	"NICK": handleNick,
	"USER": handleUser,
	// "QUIT": handleQuit,
}

// Registers the user with a unique identifier
func handleNick(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if len(params) < 1 {
		return []string{fmt.Sprintf(":%v 431 :No nickname given\r\n", server.name)}
	}

	// Check with server
	resultChan := make(chan int, 1)
	server.commandChan <- Command{NICK, params[0], make([]string, 0), resultChan}
	result := <-resultChan
	if result != OK {
		switch result {
		case ERR_NICKNAMEINUSE:
			return []string{fmt.Sprintf(":%v 433 %v :Nickname is already in use\r\n", server.name, params[0])}
		default:
			// TODO:
			return []string{}
		}
	}

	state.nick = params[0]

	// Fix this logic
	if len(state.user) > 0 && !state.registered {
		state.registered = true
		return rplWelcome(server.name, state.nick, state.user, state.host)
	}

	return []string{}
}

// Additional data about the user.
// TODO: Is this required to complete registration?
func handleUser(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if len(params) < 4 {
		return errNeedMoreParams(server.name, "USER")
	}
	if len(state.user) > 0 {
		return []string{fmt.Sprintf(":%v 462 :Unauthorized command (already registered)\r\n", server.name)}
	}

	state.user = params[0]

	if len(state.nick) > 0 && !state.registered {
		state.registered = true
		return rplWelcome(server.name, state.nick, state.user, state.host)
	}

	return []string{}
}

// End the session. Should respond and then end the connection.
func handleQuit(server *ServerInfo, state *connectionState, params []string) (response []string) {
	resultChan := make(chan int, 1)
	server.commandChan <- Command{QUIT, state.nick, make([]string, 0), resultChan}
	<-resultChan

	message := ""
	if len(params) == 0 {
		message = "Client Quit"
	} else {
		message = params[0]
	}

	return []string{fmt.Sprintf(":%v ERROR :Closing Link: %v %v\r\n", server.name, state.host, message)}
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

func rplWelcome(server string, nick string, user string, host string) []string {
	const rplWelcomeFormat = ":%v 001 %v :Welcome to the Internet Relay Network %v!%v@%v\r\n"
	return []string{fmt.Sprintf(rplWelcomeFormat, server, nick, nick, user, host)}
}

func errNeedMoreParams(server string, command string) []string {
	return []string{fmt.Sprintf(":%v 461 %v :Not enough parameters\r\n", server, command)}
}
