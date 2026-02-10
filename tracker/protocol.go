package main

type Message struct{
	Cmd 	  string  `json:"cmd"`
	Args	[]string  `json:"args"`
}

type Response struct{
	Status 		string      `json:"status"`
	Data		interface{}	`json:"data"`
}
