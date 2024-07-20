package main

import (
	"fmt"
	"strings"
)

type serverContext struct {
	info ServerInfo
	// The key is the nickname
	users       map[string]userInfo
	channels    map[string]channelInfo
	connections int
}

// TODO: rename as ServerHandle?
type ServerInfo struct {
	name string
	// Used to receive commands from the user connection
	commandChan      chan<- Command
	registrationChan chan<- Registration
}

type userInfo struct {
	user     string
	host     string
	realName string
	// Used to send messages to the user connection
	// Must be non blocking
	channel chan<- string
}

type channelInfo struct {
	members map[string]channelMember
}

type channelMember struct {
	mode byte
}

type Command struct {
	command int
	nick    string
	params  []string
	// Must be non blocking.
	responseChan chan Response
}

type Response struct {
	result int
	params string
}

type Registration struct {
	nick        string
	user        string
	host        string
	realName    string
	messageChan chan<- string
}

// Error values
const (
	OK                 = 0
	ERR_NOSUCHNICKNAME = 401
	ERR_NICKNAMEINUSE  = 433
)

func MakeServer(serverName string) (server ServerInfo) {
	commandChan := make(chan Command)
	registrationChan := make(chan Registration)

	server = ServerInfo{
		serverName,
		commandChan,
		registrationChan,
	}

	context := serverContext{
		server,
		make(map[string]userInfo),
		make(map[string]channelInfo),
		0,
	}

	go func() {
		for {
			select {
			case c := <-commandChan:
				c.responseChan <- updateData[c.command](&context, c.nick, c.params)
			case r := <-registrationChan:
				user, present := context.users[r.nick]
				if present {
					user.user = r.user
					user.host = r.host
					user.realName = r.realName
					user.channel = r.messageChan
					context.users[r.nick] = user
				}
			}
		}
	}()

	return
}

// Commands
const (
	CONNECTION_OPENED = iota
	CONNECTION_CLOSED
	NICK
	QUIT
	PRIVMSG
	N_USERS
	// N_INVISIBLE
	// N_SERVERS
	// N_OPERATORS
	N_CONNECTIONS
	// N_CHANNELS
	GET_HOST_NAME
	GET_REAL_NAME
	JOIN
)

var updateData = [](func(*serverContext, string, []string) Response){
	connectionOpened,
	connectionClosed,
	setNick,
	unregisterUser,
	privMsg,
	getNumberOfUsers,
	// getNumberOfInvisible,
	// getNumberOfOperators,
	getNumberOfConnections,
	// getNumberOfChannels
	getHostName,
	getRealName,
	userJoin,
}

func connectionOpened(context *serverContext, nick string, params []string) Response {
	context.connections += 1
	return Response{}
}
func connectionClosed(context *serverContext, nick string, params []string) Response {
	delete(context.users, nick)
	context.connections -= 1
	return Response{}
}

func setNick(context *serverContext, nick string, params []string) Response {
	// Check if nickname already registered
	_, present := context.users[nick]
	if present {
		return Response{ERR_NICKNAMEINUSE, ""}
	}

	// if not, add nickname
	context.users[nick] = userInfo{}

	return Response{}
}

func unregisterUser(context *serverContext, nick string, params []string) Response {
	delete(context.users, nick)
	return Response{}
}

func privMsg(context *serverContext, nick string, params []string) Response {
	target := params[0]
	message := params[1]

	// Check if nickname already registered
	_, present := context.users[target]
	if !present {
		return Response{ERR_NOSUCHNICKNAME, ""}
	}

	context.users[target].channel <- message

	return Response{}
}

func getNumberOfUsers(context *serverContext, nick string, params []string) Response {
	return Response{len(context.users), ""}
}

func getNumberOfConnections(context *serverContext, nick string, params []string) Response {
	return Response{context.connections, ""}
}

func getHostName(context *serverContext, nick string, params []string) Response {
	user, present := context.users[params[0]]
	if !present {
		return Response{ERR_NOSUCHNICKNAME, ""}
	}
	return Response{OK, user.host}
}

func getRealName(context *serverContext, nick string, params []string) Response {
	user, present := context.users[params[0]]
	if !present {
		return Response{ERR_NOSUCHNICKNAME, ""}
	}

	return Response{OK, user.realName}
}

func userJoin(context *serverContext, nick string, params []string) Response {
	channelName := params[0]
	member := channelMember{'+'}

	channel, present := context.channels[channelName]
	if !present {
		members := make(map[string]channelMember)
		members[nick] = member
		context.channels[channelName] = channelInfo{members}

		membersString := fmt.Sprintf("%c%v", member.mode, nick)
		return Response{OK, membersString}
	}

	for k := range channel.members {
		context.users[k].channel <- fmt.Sprintf(":%v JOIN %v\r\n", nick, params[0])
	}

	// Add member after notifying other channel members
	// to avoid unnecessarily messaging this user.
	channel.members[nick] = member

	return Response{OK, getMemberList(&channel)}
}

// utility funcs
func getMemberList(c *channelInfo) string {
	var members strings.Builder
	for k, v := range c.members {
		members.WriteByte(v.mode)
		members.WriteString(k)
		members.WriteRune(' ')
	}

	return members.String()
}
