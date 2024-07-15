package main

type IrcState struct {
	server     string
	nick       string
	user       string
	host       string
	registered bool
}

func irc_setNick(state *IrcState, nick string) (message string) {
	return ""
}

func irc_setUser(state *IrcState, user string) (message string) {
	return ""
}
