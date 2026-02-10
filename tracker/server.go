package main

import (
	"net"
	"p2p/common"
)

func handleConn(conn net.Conn) {
	defer conn.Close()

	var msg Message
	if err := common.Recv(conn, &msg); err != nil {
		return
	}

	var resp Response

	switch msg.Cmd {
	case "create_user":
		resp = createUser(msg.Args)
	case "login":
		resp = login(msg.Args)
	case "update_address":
		resp = updateAddress(msg.Args)
	case "create_group":
		resp = createGroup(msg.Args)
	case "list_requests":
		resp = listRequests(msg.Args)
	case "accept_requests":
		resp = acceptRequest(msg.Args)
	case "join_group":
		resp = joinGroup(msg.Args)
	case "upload_file":
		resp = uploadFile(msg.Args)
	case "list_files":
		resp = listFiles(msg.Args)
	case "get_file_info":
		resp = getFileInfo(msg.Args)
	case "list_groups":
		resp = listGroups(msg.Args)
	case "stop_sharing":
		resp = stopSharing(msg.Args)
	default:
		resp = Response{"error", "unkown command"}
	}

	common.Send(conn, resp)
}
