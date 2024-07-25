package main

import (
	"fmt"
	"sort"
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
	ERR_NOSUCHCHANNEL  = 403
	ERR_NICKNAMEINUSE  = 433
	ERR_NOTONCHANNEL   = 441
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
	PART
	NAMES
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
	userPart,
	getNames,
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

	targetType := target[0]
	switch targetType {
	case '&', '#', '+', '!':
		// send to channels
		channel, present := context.channels[target]
		if !present {
			return Response{ERR_NOSUCHNICKNAME, ""}
		}

		for k := range channel.members {
			context.users[k].channel <- message
		}
	default:
		// send to user
		// Check if nickname already registered
		user, present := context.users[target]
		if !present {
			return Response{ERR_NOSUCHNICKNAME, ""}
		}

		user.channel <- message
	}

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
		context.channels[channelName] = channelInfo{members}
	}
	channel, _ = context.channels[channelName]

	user, _ := context.users[nick]
	message := fmt.Sprintf(":%v!%v@%v JOIN %v\r\n", nick, user.user, user.host, params[0])

	channel.members[nick] = member
	for k := range channel.members {
		context.users[k].channel <- message
	}

	return Response{OK, getMemberList(&channel)}
}

func userPart(context *serverContext, nick string, params []string) Response {
	channelName := params[0]

	user, _ := context.users[nick]
	channel, present := context.channels[channelName]
	if !present {
		return Response{ERR_NOSUCHCHANNEL, ""}
	}

	_, present = channel.members[nick]
	if !present {
		return Response{ERR_NOTONCHANNEL, ""}
	}

	var message string
	if len(params) == 1 {
		message = fmt.Sprintf(":%v!%v@%v PART %v\r\n", nick, user.user, user.host, params[0])
	} else {
		message = fmt.Sprintf(":%v!%v@%v PART %v :%v\r\n", nick, user.user, user.host, params[0], params[1])
	}
	for k := range channel.members {
		context.users[k].channel <- message
	}

	// FIXME: Remove user from channel
	// delete(channel.members, nick)
	// FIXME: If no users left, delete channel

	return Response{}
}

func getNames(context *serverContext, nick string, params []string) Response {
	responseChan := context.users[nick].channel

	channelName := params[0]
	channel, present := context.channels[channelName]
	if !present {
		responseChan <- fmt.Sprintf(":%v 366 %v %v :End of /NAMES list\r\n", context.info.name, nick, channelName)
		return Response{ERR_NOSUCHCHANNEL, ""}
	}

	channelMembers := getMemberList(&channel)
	response := rplNames(context.info.name, nick, "=", channelName, channelMembers)

	for _, r := range response {
		responseChan <- r
	}

	return Response{OK, ""}
}

// utility funcs
func getMemberList(c *channelInfo) string {
	type memberData struct {
		name string
		mode byte
	}
	membersList := []memberData{}
	for k, v := range c.members {
		mode := v.mode
		membersList = append(membersList, memberData{k, mode})
	}
	sort.Slice(membersList, func(first, second int) bool {
		return membersList[first].name < membersList[second].name
	})

	var members strings.Builder
	for _, m := range membersList {
		members.WriteByte(m.mode)
		members.WriteString(m.name)
		members.WriteRune(' ')
	}

	return members.String()
}

func rplNames(server string, nick string, channelState string, channelName string, channelMembers string) []string {
	channelMembers = strings.TrimSpace(channelMembers)

	return []string{
		fmt.Sprintf(":%v 353 %v %v %v :%v\r\n", server, nick, channelState, channelName, channelMembers),
		fmt.Sprintf(":%v 366 %v %v :End of /NAMES list\r\n", server, nick, channelName),
	}

}
