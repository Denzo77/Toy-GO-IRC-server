package main

type serverContext struct {
	info ServerInfo
	// The key is the nickname
	users map[string]userInfo
}

type ServerInfo struct {
	name        string
	commandChan chan<- Command
}

type userInfo struct {
	user     string
	host     string
	realName string
}

type Command struct {
	command int
	nick    string
	params  []string
	// Must be non blocking.
	resultChan chan int
}

// Error values
const (
	OK                = 0
	ERR_NICKNAMEINUSE = 433
)

func MakeServer(serverName string) (server ServerInfo) {
	commandChan := make(chan Command)

	server = ServerInfo{
		serverName,
		commandChan,
	}

	context := serverContext{
		server,
		make(map[string]userInfo),
	}

	go func() {
		for c := range commandChan {
			c.resultChan <- updateData[c.command](&context, c.nick, c.params)
		}
	}()

	return
}

// Commands
const (
	NICK = iota
	USER
	QUIT
)

var updateData = [](func(*serverContext, string, []string) int){
	setNick,
	setUser,
	unregisterUser,
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
