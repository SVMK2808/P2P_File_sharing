package main

import (
	"encoding/json"
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

	case "sync_leave_group":
		if len(args) < 2 {
			return Response{"error", "sync_leave_group: need groupID, userID"}
		}
		groupID, userID := args[0], args[1]
		mu.Lock()
		defer mu.Unlock()
		if g, ok := groups[groupID]; ok {
			delete(g.Members, userID)
			fmt.Printf("[sync] %s left group %s\n", userID, groupID)
		}
		return Response{"ok", "synced"}

	case "sync_add_seeder":
		if len(args) < 3 {
			return Response{"error", "sync_add_seeder: need groupID, fileName, userID"}
		}
		groupID, fileName, userID := args[0], args[1], args[2]
		fileKey := groupID + ":" + fileName
		mu.Lock()
		defer mu.Unlock()
		if f, ok := files[fileKey]; ok {
			f.Owners[userID] = true
			fmt.Printf("[sync] %s added as seeder for %s/%s\n", userID, groupID, fileName)
		}
		return Response{"ok", "synced"}

	default:
		return Response{"error", "unknown sync command"}
	}
}

// ── Rejoin Sync ───────────────────────────────────────────────────────────────

// SyncSnapshot is the full state snapshot exchanged during tracker rejoin.
type SyncSnapshot struct {
	Users  map[string]*User  `json:"users"`
	Groups map[string]*Group `json:"groups"`
	Files  map[string]*File  `json:"files"`
}

// pullStateFromPeers is called at tracker startup (after LoadState).
// It contacts each peer in turn, requests a full state snapshot, and
// merges any missing entries into local state so the restarted tracker
// catches up with writes it missed while it was down.
func pullStateFromPeers() {
	// Give a moment for the TCP listener to be ready before dialling peers
	time.Sleep(500 * time.Millisecond)

	for _, addr := range peerAddrs {
		conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
		if err != nil {
			continue // peer is also down, try next
		}

		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if err := common.Send(conn, Message{Cmd: "sync_pull"}); err != nil {
			conn.Close()
			continue
		}

		var resp Response
		if err := common.Recv(conn, &resp); err != nil || resp.Status != "ok" {
			conn.Close()
			continue
		}
		conn.Close()

		// Unmarshal snapshot from resp.Data (JSON-encoded)
		raw, err := json.Marshal(resp.Data)
		if err != nil {
			continue
		}
		var snap SyncSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			continue
		}

		mergeState(snap)
		fmt.Printf("[rejoin] merged state from %s (%d users, %d groups, %d files)\n",
			addr, len(snap.Users), len(snap.Groups), len(snap.Files))
		return // one successful pull is enough
	}
	fmt.Println("[rejoin] no live peers found, starting with local state only")
}

// mergeState adds entries from snap that are not already present locally.
// It never overwrites existing data (last-writer-wins via broadcast handles conflicts).
func mergeState(snap SyncSnapshot) {
	mu.Lock()
	defer mu.Unlock()

	for id, u := range snap.Users {
		if _, exists := users[id]; !exists {
			users[id] = u
		}
	}
	for id, g := range snap.Groups {
		if _, exists := groups[id]; !exists {
			groups[id] = g
		}
	}
	for key, f := range snap.Files {
		if _, exists := files[key]; !exists {
			files[key] = f
		}
	}
}
