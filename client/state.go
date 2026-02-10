package main

type ClientState struct {
	UserID     string
	ListenAddr string
	Files      map[string]string // filename -> filepath
}

var State = &ClientState{}
