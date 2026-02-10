package main

import "sync"

type User struct {
	UserID   string
	Password string
	LoggedIn bool
	Addr     string
}

type Group struct {
	GroupID string
	Owner   string
	Members map[string]bool
	Pending map[string]bool
}

type Chunk struct {
	Index int    `json:"index"`
	Hash  string `json:"hash"` // SHA256 hex
	Size  int64  `json:"size"` // Bytes
}

type File struct {
	FileName    string          `json:"file_name"`
	GroupID     string          `json:"group_id"`
	Uploader    string          `json:"uploader"`
	FileSize    int64           `json:"file_size"`
	FileHash    string          `json:"file_hash"`     // SHA256 of entire file
	ChunkSize   int64           `json:"chunk_size"`    // 512KB
	TotalChunks int             `json:"total_chunks"`
	Chunks      []Chunk         `json:"chunks"`
	Owners      map[string]bool `json:"owners"`
}

var (
	users  = make(map[string]*User)
	groups = make(map[string]*Group)
	files  = make(map[string]*File)
	mu     sync.RWMutex
)
