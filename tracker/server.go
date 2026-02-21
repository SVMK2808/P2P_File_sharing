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

	// ── Sync commands from peer trackers ──────────────────────────────────────
	// These apply state locally without re-broadcasting to prevent loops.
	case "sync_create_user", "sync_create_group", "sync_join_group",
		"sync_accept_request", "sync_upload_file", "sync_stop_sharing":
		resp = applySync(msg.Cmd, msg.Args)

	// sync_pull: return full state snapshot so a restarted tracker can catch up
	case "sync_pull":
		mu.RLock()
		snap := SyncSnapshot{Users: users, Groups: groups, Files: files}
		mu.RUnlock()
		resp = Response{"ok", snap}

	default:
		resp = Response{"error", "unkown command"}
	}

	common.Send(conn, resp)
}
