package main

import (
	"encoding/json"
	"fmt"
)

func createUser(args []string) Response {
	user, pass := args[0], args[1]

	mu.Lock()
	defer mu.Unlock()

	if _, ok := users[user]; ok {
		return Response{"error", "user exists"}
	}

	users[user] = &User{
		UserID:   user,
		Password: pass,
	}

	fmt.Printf("A user with username %s has been created. ", args[0])
	go SaveState() // Persist asynchronously
	go broadcastToTrackers("sync_create_user", []string{user, pass})
	return Response{"ok", "user created"}
}

func login(args []string) Response {
	user, pass, addr := args[0], args[1], args[2]

	mu.Lock()
	defer mu.Unlock()

	u, ok := users[user]
	if !ok || u.Password != pass {
		return Response{"error", "invalid credentials"}
	}
	u.LoggedIn = true
	u.Addr = addr

	fmt.Printf("user with username = %s has logged in successfully. ", args[0])
	go SaveState() // Persist asynchronously
	return Response{"ok", "logged in"}
}

// updateAddress updates a logged-in user's peer server address
func updateAddress(args []string) Response {
	user, addr := args[0], args[1]

	mu.Lock()
	defer mu.Unlock()

	u, ok := users[user]
	if !ok {
		return Response{"error", "user not found"}
	}
	if !u.LoggedIn {
		return Response{"error", "user not logged in"}
	}

	u.Addr = addr
	fmt.Printf("Updated address for %s to %s\n", user, addr)
	go SaveState() // Persist asynchronously
	return Response{"ok", "address updated"}
}

func createGroup(args []string) Response {
	groupID, user := args[0], args[1]

	mu.Lock()
	defer mu.Unlock()

	if _, ok := groups[groupID]; ok {
		return Response{"error", "group exists"}
	}

	groups[groupID] = &Group{
		GroupID: groupID,
		Owner:   user,
		Members: map[string]bool{user: true},
		Pending: make(map[string]bool),
	}
	fmt.Printf("A group with group name = %s and group owner = %s has been created. ", groupID, user)
	go SaveState() // Persist asynchronously
	go broadcastToTrackers("sync_create_group", []string{groupID, user})
	return Response{"ok", map[string]string{
		"group_id": groupID,
		"owner":    user,
		"message":  "group created",
	}}
}

func joinGroup(args []string) Response {
	groupID, userID := args[0], args[1]

	mu.Lock()
	defer mu.Unlock()

	g, ok := groups[groupID]
	if !ok {
		return Response{"error", "group not found"}
	}

	g.Pending[userID] = true
	go broadcastToTrackers("sync_join_group", []string{groupID, userID})
	return Response{"ok", "request sent to the group"}
}

func acceptRequest(args []string) Response {
	groupID, owner, userID := args[0], args[1], args[2]

	mu.Lock()
	defer mu.Unlock()

	g := groups[groupID]
	if g.Owner != owner {
		return Response{"error", "not owner"}
	}

	delete(g.Pending, userID)
	g.Members[userID] = true
	go broadcastToTrackers("sync_accept_request", []string{groupID, userID})
	return Response{"ok", "request accepted successfully"}
}

func listRequests(args []string) Response {
	groupID, userID := args[0], args[1]

	mu.RLock()
	defer mu.RUnlock()

	g := groups[groupID]
	if g.Owner != userID {
		return Response{"error", "not owner"}
	}

	var res []string
	for u := range g.Pending {
		res = append(res, u)
	}

	return Response{"ok", res}
}

func uploadFile(args []string) Response {
	fileName, groupID, userID, fileSize := args[0], args[1], args[2], args[3]
	
	// New args: fileHash and chunksJSON (optional for backward compatibility)
	var fileHash string
	var chunks []Chunk
	
	if len(args) >= 6 {
		fileHash = args[4]
		chunksJSON := args[5]
		
		// Parse chunks from JSON
		if err := json.Unmarshal([]byte(chunksJSON), &chunks); err != nil {
			return Response{"error", "invalid chunk data"}
		}
	}

	mu.Lock()
	defer mu.Unlock()

	g, ok := groups[groupID]
	if !ok {
		return Response{"error", "group not found"}
	}

	if !g.Members[userID] {
		return Response{"error", "not a member"}
	}

	fileKey := groupID + ":" + fileName
	if _, exists := files[fileKey]; exists {
		return Response{"error", "file already exists in group"}
	}

	var size int64
	fmt.Sscanf(fileSize, "%d", &size)

	files[fileKey] = &File{
		FileName:    fileName,
		GroupID:     groupID,
		Uploader:    userID,
		FileSize:    size,
		FileHash:    fileHash,
		ChunkSize:   512 * 1024,
		TotalChunks: len(chunks),
		Chunks:      chunks,
		Owners:      map[string]bool{userID: true},
	}

	fmt.Printf("File %s uploaded to group %s by user %s\n", fileName, groupID, userID)
	if len(args) >= 6 {
		go broadcastToTrackers("sync_upload_file", args)
	}

	responseData := map[string]interface{}{
		"message":   "file uploaded successfully",
		"file_name": fileName,
		"group_id":  groupID,
		"file_size": size,
		"uploader":  userID,
	}
	
	if fileHash != "" {
		responseData["file_hash"] = fileHash
		responseData["total_chunks"] = len(chunks)
	}
	
	go SaveState() // Persist asynchronously
	
	return Response{"ok", responseData}
}

func listFiles(args []string) Response {
	groupID := args[0]

	// Optional membership check: args[1] = requesting userID
	requestingUser := ""
	if len(args) >= 2 {
		requestingUser = args[1]
	}

	mu.RLock()
	defer mu.RUnlock()

	g, ok := groups[groupID]
	if !ok {
		return Response{"error", "group not found"}
	}

	// Enforce membership when a userID is supplied
	if requestingUser != "" && !g.Members[requestingUser] {
		return Response{"error", "not a member of this group"}
	}

	var fileList []map[string]interface{}
	for _, file := range files {
		if file.GroupID == groupID {
			fileList = append(fileList, map[string]interface{}{
				"file_name": file.FileName,
				"file_size": file.FileSize,
				"uploader":  file.Uploader,
			})
		}
	}

	if len(fileList) == 0 {
		return Response{"ok", "no files in group"}
	}

	return Response{"ok", fileList}
}

// getFileInfo returns file metadata including chunks and peer list
func getFileInfo(args []string) Response {
	groupID, fileName := args[0], args[1]

	mu.RLock()
	defer mu.RUnlock()

	fileKey := groupID + ":" + fileName
	file, ok := files[fileKey]
	if !ok {
		return Response{"error", "file not found"}
	}

	return Response{"ok", map[string]interface{}{
		"file_name":    file.FileName,
		"file_hash":    file.FileHash,
		"file_size":    file.FileSize,
		"chunk_size":   file.ChunkSize,
		"total_chunks": file.TotalChunks,
		"chunks":       file.Chunks,
		"peers":        getPeerAddresses(file.Owners),
	}}
}

// getPeerAddresses returns addresses of logged-in users who own the file
func getPeerAddresses(owners map[string]bool) []string {
	var addrs []string
	for userID := range owners {
		if user, ok := users[userID]; ok && user.LoggedIn {
			addrs = append(addrs, user.Addr)
		}
	}
	return addrs
}

// listGroups returns all group IDs in the network
func listGroups(args []string) Response {
	mu.RLock()
	defer mu.RUnlock()

	if len(groups) == 0 {
		return Response{"ok", "no groups found"}
	}

	var groupList []string
	for groupID := range groups {
		groupList = append(groupList, groupID)
	}

	return Response{"ok", groupList}
}

// stopSharing removes a user from file ownership
func stopSharing(args []string) Response {
	groupID, fileName, userID := args[0], args[1], args[2]

	mu.Lock()
	defer mu.Unlock()

	fileKey := groupID + ":" + fileName
	file, ok := files[fileKey]
	if !ok {
		return Response{"error", "file not found"}
	}

	// Remove user from owners
	delete(file.Owners, userID)

	// If no owners left, delete file metadata
	if len(file.Owners) == 0 {
		delete(files, fileKey)
		fmt.Printf("File %s removed from group %s (no owners left)\n", fileName, groupID)
		go broadcastToTrackers("sync_stop_sharing", args)
		return Response{"ok", "file removed from tracker (no owners)"}
	}

	fmt.Printf("User %s stopped sharing %s in group %s\n", userID, fileName, groupID)
	go broadcastToTrackers("sync_stop_sharing", args)
	return Response{"ok", "stopped sharing"}
}

// leaveGroup removes a member from a group (owner cannot leave)
func leaveGroup(args []string) Response {
	if len(args) < 2 {
		return Response{"error", "leave_group: need groupID, userID"}
	}
	groupID, userID := args[0], args[1]

	mu.Lock()
	defer mu.Unlock()

	g, ok := groups[groupID]
	if !ok {
		return Response{"error", "group not found"}
	}
	if g.Owner == userID {
		return Response{"error", "owner cannot leave the group"}
	}
	if !g.Members[userID] {
		return Response{"error", "not a member"}
	}

	delete(g.Members, userID)
	fmt.Printf("User %s left group %s\n", userID, groupID)
	go broadcastToTrackers("sync_leave_group", args)
	go SaveState()
	return Response{"ok", "left group"}
}

// addSeeder registers an additional peer as a chunk owner for a file.
// Called by the client after successfully downloading a file.
// args: [groupID, fileName, userID]
func addSeeder(args []string) Response {
	if len(args) < 3 {
		return Response{"error", "add_seeder: need groupID, fileName, userID"}
	}
	groupID, fileName, userID := args[0], args[1], args[2]

	mu.Lock()
	defer mu.Unlock()

	fileKey := groupID + ":" + fileName
	f, ok := files[fileKey]
	if !ok {
		return Response{"error", "file not found"}
	}
	if _, isMember := groups[groupID]; !isMember {
		return Response{"error", "group not found"}
	}

	f.Owners[userID] = true
	fmt.Printf("[seeder] %s is now seeding %s in %s\n", userID, fileName, groupID)
	go broadcastToTrackers("sync_add_seeder", args)
	go SaveState()
	return Response{"ok", "registered as seeder"}
}
