package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

type connectionState struct {
	connection  net.Conn
	host        string
	nick        string
	user        string
	messageChan <-chan string
	quit        chan bool
}

func newIrcConnection(server ServerInfo, connection net.Conn) {
	state := connectionState{
		connection: connection,
		host:       connection.RemoteAddr().String(),
		nick:       "",
		user:       "",
		quit:       make(chan bool),
	}

	data := make(chan string)

	// read/write handler
	go func() {
		reader := bufio.NewReader(connection)

		for {
			// Should split on "\r\n"
			// See https://pkg.go.dev/bufio#Scanner & implementation of SplitLine
			// Could not get it to correctly handle EOF.
			netData, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			data <- netData
		}
	}()

	go func() {
		writer := bufio.NewWriter(connection)

		for {
			select {
			case netData := <-data:
				// TODO: remove pointers
				responseChan := handleIrcMessage(server, &state, netData)
				for r := range responseChan {
					writer.WriteString(r)
				}
				writer.Flush()
			case message := <-state.messageChan:
				writer.WriteString(message)
				writer.Flush()
			case <-state.quit:
				connection.Close()
				return
			}

			// fmt.Print("-> ", string(netData))
			// t := time.Now()
			// myTime := t.Format(time.RFC3339) + "\n"
		}
	}()

}

func handleIrcMessage(server ServerInfo, state *connectionState, message string) (responseChan chan string) {
	responseChan = make(chan string)

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
				state.quit <- true
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
var ircCommands = map[string](func(ServerInfo, *connectionState, []string) []string){
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
func handleNick(server ServerInfo, state *connectionState, params []string) (response []string) {
	if len(params) < 1 {
		return []string{fmt.Sprintf(":%v 431 :No nickname given\r\n", server.name)}
	}

	if isRegistered(*state) {
		// 1: already registered
		err := trySetNick(server, params[0])
		if err != nil {
			return []string{err.Error()}
		}

		oldNick := state.nick
		state.nick = params[0]
		return []string{fmt.Sprintf(":%v NICK %v\r\n", oldNick, state.nick)}
	} else if len(state.user) == 0 {
		// 2: no user details
		state.nick = params[0]
		return []string{"\r\n"}
	} else {
		return tryRegister(server, state, params[0])
	}

}

// Additional data about the user.
func handleUser(server ServerInfo, state *connectionState, params []string) (response []string) {
	if len(params) < 4 {
		return errNeedMoreParams(server.name, "USER")
	}
	if len(state.user) > 0 {
		return []string{fmt.Sprintf(":%v 462 :Unauthorized command (already registered)\r\n", server.name)}
	}

	state.user = params[0]

	if len(state.nick) == 0 {
		return []string{"\r\n"}
	} else {
		return tryRegister(server, state, state.nick)
	}
}

// End the session. Should respond and then end the connection.
func handleQuit(server ServerInfo, state *connectionState, params []string) (response []string, quit bool) {
	if !isRegistered(*state) {
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

func handlePrivmsg(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name)
	}
	if len(params) == 0 {
		return []string{fmt.Sprintf(":%v 411 %v :No recipient given (PRIVMSG)\r\n", server.name, state.nick)}
	}
	if len(params) == 1 {
		return []string{fmt.Sprintf(":%v 412 %v :No text to send\r\n", server.name, state.nick)}
	}

	message := fmt.Sprintf(":%v PRIVMSG %v :%v\r\n", state.nick, params[0], params[1])

	resultChan := make(chan int, 1)
	server.commandChan <- Command{PRIVMSG, state.nick, []string{params[0], message}, resultChan}
	result := <-resultChan
	if result == ERR_NOSUCHNICKNAME {
		return []string{fmt.Sprintf(":%v 401 %v %v :No such nick/channel\r\n", server.name, state.nick, params[0])}
	}

	return []string{"\r\n"}
}
func handleNotice(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handlePing(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handlePong(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handleMotd(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handleLusers(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}
func handleWhois(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name)
	}
	return []string{"\r\n"}
}

// func handle(server ServerInfo, state *connectionState, params []string) (response []string) {
// if !isRegistered(*state) {
// 	return errUnregistered(server.name)
// }
// 	return []string{}
// }

// utility functions
func isRegistered(state connectionState) bool {
	return len(state.nick) > 0 && len(state.user) > 0
}

func tokenize(message string) (command string, params []string) {
	message = strings.TrimSpace(message)

	// params are a space delimited list of up to 14 parameters
	// followed by an optional trailer marked with a ":"
	message, trailing, found := strings.Cut(message, " :")
	tokens := strings.Split(message, " ")

	if found {
		tokens = append(tokens, trailing)
	}

	return tokens[0], tokens[1:]
}

type nickNameInUseError struct {
	server string
	nick   string
}

func (e *nickNameInUseError) Error() string {
	return fmt.Sprintf(":%v 433 %v :Nickname is already in use\r\n", e.server, e.nick)
}

// Will clear state.nick if nickname already in use
func tryRegister(server ServerInfo, state *connectionState, nick string) []string {
	err := trySetNick(server, nick)
	if err != nil {
		state.nick = ""
		return []string{err.Error()}
	}

	state.nick = nick

	messageChan := make(chan string, 1)
	state.messageChan = messageChan
	server.registrationChan <- Registration{state.nick, state.user, messageChan}
	return rplWelcome(server.name, state.nick, state.user, state.host)
}

func trySetNick(server ServerInfo, nick string) error {
	// Check with server
	resultChan := make(chan int, 1)
	server.commandChan <- Command{NICK, nick, make([]string, 0), resultChan}
	result := <-resultChan

	switch result {
	case OK:
		return nil
	case ERR_NICKNAMEINUSE:
		return &nickNameInUseError{server.name, nick}
	default:
		// FIXME: This should still be an error case
		panic(0)
	}
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
