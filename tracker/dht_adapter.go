package main

import (
	"fmt"
	"log"
	"p2p/dht"
)

// TrackerDHT wraps DHT client for tracker use
type TrackerDHT struct {
	client *dht.P2PClient
}

var trackerDHT *TrackerDHT

// InitTrackerDHT initializes DHT for this tracker
func InitTrackerDHT(trackerID string, port int, peerAddrs []string) error {
	config := &dht.Config{
		NodeID:            "tracker_" + trackerID,
		Host:              "127.0.0.1",
		Port:              port + 1000, // DHT port = tracker port + 1000
		Peers:             loadTrackerPeers(peerAddrs),
		ReplicationFactor: 3,
		ReadQuorum:        2,
		WriteQuorum:       2,
	}
	
	client, err := dht.NewP2PClient(config, peerAddrs)
	if err != nil {
		return fmt.Errorf("failed to create DHT client: %v", err)
	}
	
	if err := client.Start(); err != nil {
		return fmt.Errorf("failed to start DHT: %v", err)
	}
	
	trackerDHT = &TrackerDHT{client: client}
	log.Printf("Tracker DHT initialized on port %d\n", port+1000)
	
	return nil
}

// loadTrackerPeers converts peer addresses to DHT peer configs
func loadTrackerPeers(peerAddrs []string) []dht.PeerConfig {
	peers := make([]dht.PeerConfig, 0)
	for i, addr := range peerAddrs {
		peers = append(peers, dht.PeerConfig{
			NodeID: fmt.Sprintf("tracker_%d", i+1),
			Host:   "127.0.0.1",
			Port:   extractPort(addr) + 1000,
		})
	}
	return peers
}

// extractPort extracts port from address string ":9000" -> 9000
func extractPort(addr string) int {
	var port int
	fmt.Sscanf(addr, ":%d", &port)
	return port
}

// StopTrackerDHT stops the DHT client
func StopTrackerDHT() error {
	if trackerDHT != nil && trackerDHT.client != nil {
		return trackerDHT.client.Stop()
	}
	return nil
}

// ===== DHT Operations =====

// PutUser stores user in DHT
func (t *TrackerDHT) PutUser(userID string, user *User) error {
	key := "user:" + userID
	value := map[string]interface{}{
		"password":  user.Password,
		"logged_in": user.LoggedIn,
		"addr":      user.Addr,
	}
	return t.client.Put(key, value)
}

// GetUser retrieves user from DHT
func (t *TrackerDHT) GetUser(userID string) (*User, error) {
	userData, err := t.client.GetUser(userID)
	if err != nil {
		return nil, err
	}
	
	user := &User{
		UserID: userID,
	}
	if pass, ok := userData["password"].(string); ok {
		user.Password = pass
	}
	if loggedIn, ok := userData["logged_in"].(bool); ok {
		user.LoggedIn = loggedIn
	}
	if addr, ok := userData["addr"].(string); ok {
		user.Addr = addr
	}
	
	return user, nil
}

// PutGroup stores group in DHT
func (t *TrackerDHT) PutGroup(groupID string, group *Group) error {
	return t.client.CreateGroup(groupID, group.Owner)
}

// GetGroup retrieves group from DHT
func (t *TrackerDHT) GetGroup(groupID string) (*Group, error) {
	groupData, err := t.client.GetGroup(groupID)
	if err != nil {
		return nil, err
	}
	
	group := &Group{
		GroupID: groupID,
		Members: make(map[string]bool),
		Pending: make(map[string]bool),
	}
	if owner, ok := groupData["owner"].(string); ok {
		group.Owner = owner
		group.Members[owner] = true
	}
	
	return group, nil
}

// PutFile stores file metadata in DHT
func (t *TrackerDHT) PutFile(fileKey string, file *File) error {
	metadata := &dht.FileMetadata{
		FileName:    file.FileName,
		GroupID:     file.GroupID,
		Uploader:    file.Uploader,
		FileSize:    file.FileSize,
		FileHash:    file.FileHash,
		ChunkSize:   file.ChunkSize,
		TotalChunks: file.TotalChunks,
		Chunks:      convertChunks(file.Chunks),
	}
	return t.client.UploadFile(metadata)
}

// convertChunks converts tracker Chunk to DHT ChunkInfo
func convertChunks(chunks []Chunk) []dht.ChunkInfo {
	dhtChunks := make([]dht.ChunkInfo, len(chunks))
	for i, c := range chunks {
		dhtChunks[i] = dht.ChunkInfo{
			Index: c.Index,
			Hash:  c.Hash,
			Size:  c.Size,
		}
	}
	return dhtChunks
}

// ListGroups returns all groups from DHT
func (t *TrackerDHT) ListGroups() ([]string, error) {
	return t.client.ListGroups()
}

// ListFiles returns all files in a group from DHT
func (t *TrackerDHT) ListFiles(groupID string) ([]string, error) {
	return t.client.ListFiles(groupID)
}
