package dht

import (
	"encoding/json"
	"fmt"
	"strings"
)

// P2PClient wraps Coordinator for P2P-specific operations
type P2PClient struct {
	*Coordinator
}

// NewP2PClient creates a new P2P DHT client
func NewP2PClient(config *Config, peerAddrs []string) (*P2PClient, error) {
	// Create consistent hash ring
	ring := NewConsistentHashRing()
	
	// Add all peers to ring
	allNodes := make([]string, 0)
	for _, peer := range config.Peers {
		nodeAddr := fmt.Sprintf("%s:%d", peer.Host, peer.Port)
		allNodes = append(allNodes, nodeAddr)
		ring.AddNode(nodeAddr)
	}
	
	// Add self
	selfAddr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	ring.AddNode(selfAddr)
	allNodes = append(allNodes, selfAddr)
	
	// Create gossip service
	gossip := NewGossipService(config.NodeID, allNodes)
	
	// Create coordinator (NewNode inside will create storage)
	coordinator := NewCoordinator(
		config.NodeID,
		ring,
		config.ReplicationFactor,
		config.ReadQuorum,
		config.WriteQuorum,
	)
	coordinator.Gossip = gossip
	
	return &P2PClient{Coordinator: coordinator}, nil
}

// Start starts the DHT client
func (c *P2PClient) Start() error {
	// Start gossip
	c.Gossip.Start()
	
	// TODO: Start HTTP server for DHT endpoints if needed
	return nil
}

// Stop stops the DHT client
func (c *P2PClient) Stop() error {
	c.Gossip.Stop()
	return c.Storage.Close()
}

// ===== User Operations =====

// CreateUser stores user data in DHT
func (c *P2PClient) CreateUser(username, password string) error {
	key := "user:" + username
	value := map[string]interface{}{
		"password": password,
		"logged_in": false,
		"addr": "",
	}
	return c.Put(key, value)
}

// GetUser retrieves user data from DHT
func (c *P2PClient) GetUser(username string) (map[string]interface{}, error) {
	key := "user:" + username
	result, err := c.Get(key)
	if err != nil {
		return nil, err
	}
	
	if userData, ok := result["value"].(map[string]interface{}); ok {
		return userData, nil
	}
	return nil, fmt.Errorf("user not found")
}

// UpdateUserLogin updates user login status and peer address
func (c *P2PClient) UpdateUserLogin(username, peerAddr string, loggedIn bool) error {
	key := "user:" + username
	value := map[string]interface{}{
		"addr": peerAddr,
		"logged_in": loggedIn,
	}
	// Note: This will merge with existing user data via vector clocks
	return c.Put(key, value)
}

// ===== Group Operations =====

// CreateGroup stores group data in DHT
func (c *P2PClient) CreateGroup(groupID, owner string) error {
	key := "group:" + groupID
	value := map[string]interface{}{
		"owner": owner,
		"members": []string{owner},
	}
	return c.Put(key, value)
}

// GetGroup retrieves group data from DHT
func (c *P2PClient) GetGroup(groupID string) (map[string]interface{}, error) {
	key := "group:" + groupID
 result, err := c.Get(key)
	if err != nil {
		return nil, err
	}
	
	if groupData, ok := result["value"].(map[string]interface{}); ok {
		return groupData, nil
	}
	return nil, fmt.Errorf("group not found")
}

// ListGroups returns all groups (iterates DHT)
func (c *P2PClient) ListGroups() ([]string, error) {
	groups := make([]string, 0)
	
	c.Storage.Iterate(func(key string, value storedValue) bool {
		if strings.HasPrefix(key, "group:") {
			groupID := strings.TrimPrefix(key, "group:")
			groups = append(groups, groupID)
		}
		return true // Continue iteration
	})
	
	return groups, nil
}

// ===== File Operations =====

// FileMetadata represents file information
type FileMetadata struct {
	FileName    string      `json:"file_name"`
	GroupID     string      `json:"group_id"`
	Uploader    string      `json:"uploader"`
	FileSize    int64       `json:"file_size"`
	FileHash    string      `json:"file_hash"`
	ChunkSize   int64       `json:"chunk_size"`
	TotalChunks int         `json:"total_chunks"`
	Chunks      []ChunkInfo `json:"chunks"`
}

// ChunkInfo represents chunk metadata
type ChunkInfo struct {
	Index int    `json:"index"`
	Hash  string `json:"hash"`
	Size  int64  `json:"size"`
}

// UploadFile stores file metadata in DHT
func (c *P2PClient) UploadFile(metadata *FileMetadata) error {
	key := fmt.Sprintf("file:%s:%s", metadata.GroupID, metadata.FileName)
	return c.Put(key, metadata)
}

// GetFileInfo retrieves file metadata from DHT
func (c *P2PClient) GetFileInfo(groupID, fileName string) (*FileMetadata, error) {
	key := fmt.Sprintf("file:%s:%s", groupID, fileName)
	result, err := c.Get(key)
	if err != nil {
		return nil, err
	}
	
	// Convert map to FileMetadata
	jsonData, err := json.Marshal(result["value"])
	if err != nil {
		return nil, err
	}
	
	var metadata FileMetadata
	if err := json.Unmarshal(jsonData, &metadata); err != nil {
		return nil, err
	}
	
	return &metadata, nil
}

// ListFiles returns all files in a group
func (c *P2PClient) ListFiles(groupID string) ([]string, error) {
	files := make([]string, 0)
	prefix := "file:" + groupID + ":"
	
	c.Storage.Iterate(func(key string, value storedValue) bool {
		if strings.HasPrefix(key, prefix) {
			fileName := strings.TrimPrefix(key, prefix)
			files = append(files, fileName)
		}
		return true
	})
	
	return files, nil
}

// ===== Chunk Availability Operations =====

// AnnounceChunk announces that this peer has a specific chunk
func (c *P2PClient) AnnounceChunk(fileHash string, chunkIndex int, peerAddr string) error {
	key := fmt.Sprintf("chunk:%s:%d", fileHash, chunkIndex)
	
	// Get existing peer list
	existingPeers := make([]string, 0)
	result, err := c.Get(key)
	if err == nil {
		if peers, ok := result["value"].([]interface{}); ok {
			for _, p := range peers {
				if peerStr, ok := p.(string); ok {
					existingPeers = append(existingPeers, peerStr)
				}
			}
		}
	}
	
	// Add this peer if not already in list
	found := false
	for _, p := range existingPeers {
		if p == peerAddr {
			found = true
			break
		}
	}
	
	if !found {
		existingPeers = append(existingPeers, peerAddr)
	}
	
	// Update DHT
	return c.Put(key, existingPeers)
}

// GetChunkPeers returns list of peers that have a specific chunk
func (c *P2PClient) GetChunkPeers(fileHash string, chunkIndex int) ([]string, error) {
	key := fmt.Sprintf("chunk:%s:%d", fileHash, chunkIndex)
	result, err := c.Get(key)
	if err != nil {
		return []string{}, nil // No peers have this chunk yet
	}
	
	peers := make([]string, 0)
	if peerList, ok := result["value"].([]interface{}); ok {
		for _, p := range peerList {
			if peerStr, ok := p.(string); ok {
				peers = append(peers, peerStr)
			}
		}
	}
	
	return peers, nil
}

// GetChunkAvailability returns a map of chunk index -> peer count
func (c *P2PClient) GetChunkAvailability(fileHash string, totalChunks int) map[int]int {
	availability := make(map[int]int)
	
	for i := 0; i < totalChunks; i++ {
		peers, _ := c.GetChunkPeers(fileHash, i)
		availability[i] = len(peers)
	}
	
	return availability
}
