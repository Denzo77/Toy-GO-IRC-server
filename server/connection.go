package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

type connectionState struct {
	connection net.Conn
	nick       string
	user       string
	host       string
	registered bool
}

func newIrcConnection(server ServerInfo, connection net.Conn) {
	state := connectionState{
		connection: connection,
		host:       connection.RemoteAddr().String(),
	}

	go func() {
		reader := bufio.NewReader(connection)
		writer := bufio.NewWriter(connection)

		for {

			// Should split on "\r\n"
			// See https://pkg.go.dev/bufio#Scanner & implementation of SplitLine
			// Could not get it to correctly handle EOF.
			netData, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			// TODO: remove pointers
			responseChan, quitChan := handleIrcMessage(&server, &state, netData)

			for r := range responseChan {
				writer.WriteString(r)
			}
			writer.Flush()

			// fmt.Print("-> ", string(netData))
			// t := time.Now()
			// myTime := t.Format(time.RFC3339) + "\n"
			select {
			case <-quitChan:
				connection.Close()
				return
			default:
				continue
			}
		}
	}()

}

func handleIrcMessage(server *ServerInfo, state *connectionState, message string) (responseChan chan string, quitChan chan bool) {
	responseChan = make(chan string)
	quitChan = make(chan bool)

	respondMultiple := func(response []string) {
		for _, r := range response {
			responseChan <- r
		}
	}

	go func() {

		command, params := tokenize(message)

		// Slightly hacky special case to avoid editing all command handlers
		// TODO: May need to change anyway in the future.
		if command == "QUIT" {
			response, quit := handleQuit(server, state, params)
			respondMultiple(response)
			close(responseChan)
			if quit {
				quitChan <- true
			}
			return
		}

		handler, valid_command := ircCommands[command]
		if !valid_command {
			responseChan <- fmt.Sprintf(":%v 421 %v :Unknown command\r\n", server.name, command)
			close(responseChan)
			return
		}

		respondMultiple(handler(server, state, params))
		close(responseChan)
		return
	}()

	return
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
			return []string{"\r\n"}
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
		return []string{"\r\n"}
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

	return []string{"\r\n"}
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
	return []string{"\r\n"}
}
func handleNotice(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handlePing(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handlePong(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handleMotd(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handleLusers(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handleWhois(server *ServerInfo, state *connectionState, params []string) (response []string) {
	if !state.registered {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
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
