package main

type serverContext struct {
	info ServerInfo
	// The key is the nickname
	users map[string]userInfo
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

type Command struct {
	command int
	nick    string
	params  []string
	// Must be non blocking.
	resultChan chan int
}

type Registration struct {
	nick        string
	user        string
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
	}

	go func() {
		for {
			select {
			case c := <-commandChan:
				c.resultChan <- updateData[c.command](&context, c.nick, c.params)
			case r := <-registrationChan:
				user, present := context.users[r.nick]
				if present {
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
	NICK = iota
	USER
	QUIT
	PRIVMSG
)

var updateData = [](func(*serverContext, string, []string) int){
	setNick,
	setUser,
	unregisterUser,
	privMsg,
}

func setNick(context *serverContext, nick string, params []string) int {
	// Check if nickname already registered
	_, present := context.users[nick]
	if present {
		return ERR_NICKNAMEINUSE
	}

	// if not, add nickname
	context.users[nick] = userInfo{}

	return OK
}

func setUser(context *serverContext, nick string, params []string) int {
	return OK
}

func unregisterUser(context *serverContext, nick string, params []string) int {
	delete(context.users, nick)

	return OK
}

func privMsg(context *serverContext, nick string, params []string) int {
	target := params[0]
	message := params[1]

	// Check if nickname already registered
	_, present := context.users[target]
	if !present {
		return ERR_NOSUCHNICKNAME
	}

	context.users[target].channel <- message

	return OK
}
