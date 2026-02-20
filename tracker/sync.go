package main

import (
	"fmt"
	"net"
	"p2p/common"
	"time"
)

// peerAddrs holds the TCP addresses of all other trackers (set at startup)
var peerAddrs []string

// broadcastToTrackers fans out a sync command to all peer trackers asynchronously.
// It skips trackers that are unreachable — they will receive the state on restart
// via the persisted SaveState/LoadState mechanism.
func broadcastToTrackers(cmd string, args []string) {
	msg := Message{Cmd: cmd, Args: args}
	for _, addr := range peerAddrs {
		go func(target string) {
			conn, err := net.DialTimeout("tcp", target, 500*time.Millisecond)
			if err != nil {
				// Peer is down — not an error, skip silently
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			if err := common.Send(conn, msg); err != nil {
				return
			}
			// Read (and discard) the ack so the peer's handleConn completes cleanly
			var resp Response
			common.Recv(conn, &resp)
		}(addr)
	}
}

// applySync applies an inbound sync message to local in-memory state
// WITHOUT re-broadcasting (prevents infinite loops).
func applySync(cmd string, args []string) Response {
	switch cmd {
	case "sync_create_user":
		if len(args) < 2 {
			return Response{"error", "sync_create_user: need user, pass"}
		}
		user, pass := args[0], args[1]
		mu.Lock()
		defer mu.Unlock()
		if _, exists := users[user]; !exists {
			users[user] = &User{UserID: user, Password: pass}
			fmt.Printf("[sync] created user %s\n", user)
		}
		return Response{"ok", "synced"}

	case "sync_create_group":
		if len(args) < 2 {
			return Response{"error", "sync_create_group: need groupID, owner"}
		}
		groupID, owner := args[0], args[1]
		mu.Lock()
		defer mu.Unlock()
		if _, exists := groups[groupID]; !exists {
			groups[groupID] = &Group{
				GroupID: groupID,
				Owner:   owner,
				Members: map[string]bool{owner: true},
				Pending: make(map[string]bool),
			}
			fmt.Printf("[sync] created group %s\n", groupID)
		}
		return Response{"ok", "synced"}

	case "sync_join_group":
		if len(args) < 2 {
			return Response{"error", "sync_join_group: need groupID, userID"}
		}
		groupID, userID := args[0], args[1]
		mu.Lock()
		defer mu.Unlock()
		if g, ok := groups[groupID]; ok {
			g.Pending[userID] = true
			fmt.Printf("[sync] %s pending in group %s\n", userID, groupID)
		}
		return Response{"ok", "synced"}

	case "sync_accept_request":
		if len(args) < 2 {
			return Response{"error", "sync_accept_request: need groupID, userID"}
		}
		groupID, userID := args[0], args[1]
		mu.Lock()
		defer mu.Unlock()
		if g, ok := groups[groupID]; ok {
			delete(g.Pending, userID)
			g.Members[userID] = true
			fmt.Printf("[sync] accepted %s into group %s\n", userID, groupID)
		}
		return Response{"ok", "synced"}

	case "sync_upload_file":
		// args: fileName, groupID, userID, fileSize, fileHash, chunksJSON
		if len(args) < 6 {
			return Response{"error", "sync_upload_file: insufficient args"}
		}
		// Reuse the existing uploadFile handler (it's idempotent for new files)
		resp := uploadFile(args)
		fmt.Printf("[sync] upload_file result: %s\n", resp.Status)
		return Response{"ok", "synced"}

	case "sync_stop_sharing":
		if len(args) < 3 {
			return Response{"error", "sync_stop_sharing: need groupID, fileName, userID"}
		}
		resp := stopSharing(args)
		fmt.Printf("[sync] stop_sharing result: %s\n", resp.Status)
		return Response{"ok", "synced"}

	default:
		return Response{"error", "unknown sync command"}
	}
}
