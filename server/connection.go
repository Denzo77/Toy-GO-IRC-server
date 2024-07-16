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
		return handleQuit(server, state, params)
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
	"PRIVMSG": handlePrivmsg,
	"NOTICE":  handleNotice,
	"PING":    handlePing,
	"PONG":    handlePong,
	"MOTD":    handleMotd,
	"LUSERS":  handleLusers,
	"WHOIS":   handleWhois,
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

	oldNick := state.nick
	state.nick = params[0]

	// Fix this logic
	if len(state.user) > 0 && !state.registered {
		state.registered = true
		return rplWelcome(server.name, state.nick, state.user, state.host)
	}

	if len(oldNick) == 0 {
		return []string{}
	} else {
		return []string{fmt.Sprintf(":%v NICK %v\r\n", oldNick, state.nick)}
	}
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
func handleQuit(server *ServerInfo, state *connectionState, params []string) (response []string, quit bool) {
	if !state.registered {
		return errUnregistered(server.name), false
	}

	resultChan := make(chan int, 1)
	server.commandChan <- Command{QUIT, state.nick, make([]string, 0), resultChan}
	<-resultChan

	message := ""
	if len(params) == 0 {
		message = "Client Quit"
	} else {
		message = params[0]
	}

	return []string{fmt.Sprintf(":%v ERROR :Closing Link: %v %v\r\n", server.name, state.host, message)}, true
}

func handlePrivmsg(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{}
}
func handleNotice(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{}
}
func handlePing(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{}
}
func handlePong(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{}
}
func handleMotd(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{}
}
func handleLusers(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{}
}
func handleWhois(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{}
}

// func handle(server *ServerInfo, state *connectionState, params []string) (response []string) {
// if !state.registered {
// 	return errUnregistered(server.name)
// }
// 	return []string{}
// }

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
	// FIXME:
	const version = "0.0"
	const creationDate = "01/01/1970"
	const userModes = "0"
	const channelModes = "0"
	const rplWelcomeFormat = ":%v 001 %v :Welcome to the Internet Relay Network %v!%v@%v\r\n"
	// TODO! Add MOTD and LUSER responses
	return []string{
		fmt.Sprintf(rplWelcomeFormat, server, nick, nick, user, host),
		fmt.Sprintf(":%v 002 %v :Your host is %v, running version %v\r\n", server, nick, server, version),
		fmt.Sprintf(":%v 003 %v :This server was created %v\r\n", server, nick, creationDate),
		fmt.Sprintf(":%v 004 %v :%v %v %v %v\r\n", server, nick, server, version, userModes, channelModes),
	}
}

func errNeedMoreParams(server string, command string) []string {
	return []string{fmt.Sprintf(":%v 461 %v :Not enough parameters\r\n", server, command)}
}

func errUnregistered(server string) []string {
	return []string{fmt.Sprintf(":%v 451 :You have not registered\r\n", server)}
}
