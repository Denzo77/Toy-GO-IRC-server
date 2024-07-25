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
	realName    string
	messageChan chan string
	quit        chan bool
}

func newIrcConnection(server ServerInfo, connection net.Conn) {
	state := connectionState{
		connection:  connection,
		host:        connection.RemoteAddr().String(),
		nick:        "",
		user:        "",
		realName:    "",
		messageChan: make(chan string, 1),
		quit:        make(chan bool),
	}

	sendCommandToServer(server.commandChan, CONNECTION_OPENED, "", []string{})

	// read/write handler
	// TODO: Check this quits correctly
	go func() {
		reader := bufio.NewReader(connection)

		for {
			// Should split on "\r\n"
			// See https://pkg.go.dev/bufio#Scanner & implementation of SplitLine
			// Could not get it to correctly handle EOF.
			netData, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println(err.Error())
				state.quit <- true
				return
			}

			// TODO: remove pointers
			_ = handleIrcMessage(server, &state, netData)
			// for r := range responseChan {
			// 	state.messageChan <- r
			// }
		}
	}()

	go func() {
		// TODO: This is poorly tested
		defer sendCommandToServer(server.commandChan, CONNECTION_CLOSED, state.nick, []string{})

		writer := bufio.NewWriter(connection)

		for {
			select {
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
	respondMultiple := func(channel chan<- string, response []string) {
		for _, r := range response {
			channel <- r
		}
	}

	go func() {
		command, params := tokenize(message)

		// Slightly hacky special case to avoid editing all command handlers
		// TODO: May need to change anyway in the future.
		if command == "QUIT" {
			response, quit := handleQuit(server, state, params)
			respondMultiple(state.messageChan, response)
			if quit {
				state.quit <- true
			}
			return
		}

		handler, valid_command := ircCommands[command]
		if !valid_command {
			nick := state.nick
			if len(nick) == 0 {
				nick = "*"
			}
			state.messageChan <- fmt.Sprintf(":%v 421 %v %v :Unknown command\r\n", server.name, nick, command)
			return
		}

		respondMultiple(state.messageChan, handler(server, state, params))
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
	"JOIN":    handleJoin,
	"PART":    handlePart,
	"TOPIC":   handleTopic,
	"AWAY":    handleAway,
	"NAMES":   handleNames,
	"LIST":    handleList,
	"WHO":     handleWho,
}

// Registers the user with a unique identifier
func handleNick(server ServerInfo, state *connectionState, params []string) (response []string) {
	if len(params) < 1 {
		nick := state.nick
		if !isRegistered(*state) {
			nick = "*"
		}
		return []string{fmt.Sprintf(":%v 431 %v :No nickname given\r\n", server.name, nick)}
	}

	if isRegistered(*state) {
		// 1: already registered
		err := trySetNick(server, state.nick, params[0])
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
	nick := state.nick
	if len(nick) == 0 {
		nick = "*"
	}

	if len(params) < 4 {
		return errNeedMoreParams(server.name, nick, "USER")
	}
	if len(state.user) > 0 {
		return []string{fmt.Sprintf(":%v 462 %v :Unauthorized command (already registered)\r\n", server.name, nick)}
	}

	state.user = params[0]
	state.realName = params[3]

	if len(state.nick) == 0 {
		return []string{"\r\n"}
	} else {
		return tryRegister(server, state, nick)
	}
}

// End the session. Should respond and then end the connection.
func handleQuit(server ServerInfo, state *connectionState, params []string) (response []string, quit bool) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick), false
	}

	sendCommandToServer(server.commandChan, QUIT, state.nick, []string{})

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
		return errUnregistered(server.name, state.nick)
	}
	if len(params) == 0 {
		return []string{fmt.Sprintf(":%v 411 %v :No recipient given (PRIVMSG)\r\n", server.name, state.nick)}
	}
	if len(params) == 1 {
		return []string{fmt.Sprintf(":%v 412 %v :No text to send\r\n", server.name, state.nick)}
	}

	message := fmt.Sprintf(":%v!%v@%v PRIVMSG %v :%v\r\n", state.nick, state.user, state.host, params[0], params[1])
	result, _ := sendCommandToServer(server.commandChan, PRIVMSG, state.nick, []string{params[0], message})

	if result == ERR_NOSUCHNICKNAME {
		return []string{fmt.Sprintf(":%v 401 %v %v :No such nick/channel\r\n", server.name, state.nick, params[0])}
	}

	return []string{"\r\n"}
}
func handleNotice(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}
	if len(params) < 2 {
		return []string{"\r\n"}
	}

	message := fmt.Sprintf(":%v!%v@%v NOTICE %v :%v\r\n", state.nick, state.user, state.host, params[0], params[1])
	sendCommandToServer(server.commandChan, PRIVMSG, state.nick, []string{params[0], message})

	return []string{"\r\n"}
}
func handlePing(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}

	if len(params) < 1 {
		return errNeedMoreParams(server.name, state.nick, "PING")
	}

	return []string{fmt.Sprintf(":%v PONG %v %v\r\n", server.name, server.name, params[0])}
}
func handlePong(server ServerInfo, state *connectionState, params []string) (response []string) {
	// TODO: Should we actually do this check?
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}
	return []string{"\r\n"}
}
func handleMotd(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}
	return []string{fmt.Sprintf(":%v 422 %v :MOTD not implemented\r\n", server.name, state.nick)}
}
func handleLusers(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}

	clients, _ := sendCommandToServer(server.commandChan, N_CONNECTIONS, state.nick, params)
	users, _ := sendCommandToServer(server.commandChan, N_USERS, state.nick, params)
	invisible := 0
	servers := 0
	operators := 0
	unknown := clients - users
	channels := 0
	// FIXME: Should this be users + unknown + invisible?
	return []string{
		fmt.Sprintf(":%v 251 %v :There are %v users and %v invisible on %v servers\r\n", server.name, state.nick, users, invisible, servers),
		fmt.Sprintf(":%v 252 %v %v :operator(s) online\r\n", server.name, state.nick, operators),
		fmt.Sprintf(":%v 253 %v %v :unknown connection(s)\r\n", server.name, state.nick, unknown),
		fmt.Sprintf(":%v 254 %v %v :channels formed\r\n", server.name, state.nick, channels),
		fmt.Sprintf(":%v 255 %v :I have %v clients and %v servers\r\n", server.name, state.nick, clients, servers),
	}
}
func handleWhois(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}
	if len(params) < 1 {
		return []string{"\r\n"}
	}

	targetNick := params[0]
	result, targetHost := sendCommandToServer(server.commandChan, GET_HOST_NAME, state.nick, params[:1])
	if result == ERR_NOSUCHNICKNAME {
		return []string{fmt.Sprintf(":%v 401 %v %v :No such nick/channel\r\n", server.name, state.nick, params[0])}
	}
	_, targetName := sendCommandToServer(server.commandChan, GET_REAL_NAME, state.nick, params[:1])

	return []string{
		fmt.Sprintf(":%v 311 %v %v %v %v :%v\r\n", server.name, state.nick, targetNick, targetNick, targetHost, targetName),
		fmt.Sprintf(":%v 312 %v %v %v :Toy server\r\n", server.name, state.nick, targetNick, server.name),
		fmt.Sprintf(":%v 318 %v %v :End of /WHOIS list\r\n", server.name, state.nick, targetNick),
	}
}

func handleJoin(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}
	if len(params) < 1 {
		return errNeedMoreParams(server.name, state.nick, "JOIN")
	}

	channelName := params[0]

	_, channelMembers := sendCommandToServer(server.commandChan, JOIN, state.nick, params[:1])

	channelMembers = strings.TrimSpace(channelMembers)

	response = []string{
		fmt.Sprintf(":%v 332 %v %v :Test\r\n", server.name, state.nick, channelName),
	}
	response = append(response, rplNames(server.name, state.nick, "=", channelName, channelMembers)...)

	return
}
func handlePart(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}
	if len(params) < 1 {
		return errNeedMoreParams(server.name, state.nick, "PART")
	}
	result, _ := sendCommandToServer(server.commandChan, PART, state.nick, params)

	channel := params[0]
	if result == ERR_NOSUCHCHANNEL {
		return []string{fmt.Sprintf(":%v 403 %v %v :No such channel\r\n", server.name, state.nick, channel)}
	} else if result == ERR_NOTONCHANNEL {
		return []string{fmt.Sprintf(":%v 441 %v %v :You're not on that channel\r\n", server.name, state.nick, channel)}
	}

	return []string{"\r\n"}
}
func handleTopic(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}

	return []string{"\r\n"}
}
func handleAway(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}

	return []string{"\r\n"}
}

func handleNames(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}

	_, _ = sendCommandToServer(server.commandChan, NAMES, state.nick, params)

	return []string{"\r\n"}
}

func handleList(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
	}

	return []string{"\r\n"}
}
func handleWho(server ServerInfo, state *connectionState, params []string) (response []string) {
	if !isRegistered(*state) {
		return errUnregistered(server.name, state.nick)
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
	client string
	nick   string
}

func (e *nickNameInUseError) Error() string {
	return fmt.Sprintf(":%v 433 %v %v :Nickname is already in use\r\n", e.server, e.client, e.nick)
}

// Will clear state.nick if nickname already in use
func tryRegister(server ServerInfo, state *connectionState, nick string) []string {
	err := trySetNick(server, "*", nick)
	if err != nil {
		state.nick = ""
		return []string{err.Error()}
	}

	state.nick = nick

	server.registrationChan <- Registration{state.nick, state.user, state.host, state.realName, state.messageChan}
	return rplWelcome(server.name, state.nick, state.user, state.host)
}

func trySetNick(server ServerInfo, client, nick string) error {
	// Check with server
	result, _ := sendCommandToServer(server.commandChan, NICK, nick, []string{})
	switch result {
	case OK:
		return nil
	case ERR_NICKNAMEINUSE:
		return &nickNameInUseError{server.name, client, nick}
	default:
		// FIXME: This should still be an error case
		panic(0)
	}
}

func sendCommandToServer(channel chan<- Command, command int, nick string, params []string) (result int, response string) {
	resultChan := make(chan Response, 1)
	channel <- Command{command, nick, params, resultChan}
	r := <-resultChan
	return r.result, r.params
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

func errNeedMoreParams(server string, nick string, command string) []string {
	return []string{fmt.Sprintf(":%v 461 %v %v :Not enough parameters\r\n", server, nick, command)}
}

func errUnregistered(server string, nick string) []string {
	return []string{fmt.Sprintf(":%v 451 %v :You have not registered\r\n", server, nick)}
}
