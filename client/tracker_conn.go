package main

import (
	"net"
	"p2p/common"
)

func SendToTracker(msg Message) Response {
	conn, err := net.Dial("tcp", "127.0.0.1:9000")
	if err != nil {
		return Response{"error", "could not connect to tracker"}
	}
	defer conn.Close()

	common.Send(conn, msg)

	var resp Response
	common.Recv(conn, &resp)
	return resp
}
