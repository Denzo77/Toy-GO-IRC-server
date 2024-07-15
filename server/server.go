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
	command    int
	nick       string
	params     []string
	resultChan chan int
}

// Commands
const (
	NICK = iota
	USER
)

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
			// updateData[c.command](c.params)
			c.resultChan <- setNick(&context, c.nick, c.params)
		}
	}()

	return
}

// var updateData := []func(){

// }
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
