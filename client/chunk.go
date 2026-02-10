package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	ChunkSize = 512 * 1024 // 512KB
	ChunksDir = ".chunks"
)

// ChunkInfo represents metadata for a single chunk
type ChunkInfo struct {
	Index int    `json:"index"`
	Hash  string `json:"hash"` // SHA256 hex
	Size  int64  `json:"size"` // Bytes
}

// ChunkMetadata contains all metadata for a chunked file
type ChunkMetadata struct {
	FileName    string      `json:"file_name"`
	FileSize    int64       `json:"file_size"`
	FileHash    string      `json:"file_hash"`    // SHA256 of entire file
	ChunkSize   int64       `json:"chunk_size"`   // 512KB
	TotalChunks int         `json:"total_chunks"`
	Chunks      []ChunkInfo `json:"chunks"`
}

// CalculateFileHash calculates SHA256 hash of entire file
func CalculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ChunkFile splits a file into chunks and calculates hashes
func ChunkFile(filePath string) (*ChunkMetadata, error) {
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	// Calculate total chunks
	fileSize := fileInfo.Size()
	totalChunks := int((fileSize + ChunkSize - 1) / ChunkSize)

	// Calculate file hash
	fileHash, err := CalculateFileHash(filePath)
	if err != nil {
		return nil, err
	}

	// Open file for reading
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create metadata
	metadata := &ChunkMetadata{
		FileName:    filepath.Base(filePath),
		FileSize:    fileSize,
		FileHash:    fileHash,
		ChunkSize:   ChunkSize,
		TotalChunks: totalChunks,
		Chunks:      make([]ChunkInfo, 0, totalChunks),
	}

	// Read and hash each chunk
	buffer := make([]byte, ChunkSize)
	for i := 0; i < totalChunks; i++ {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, err
		}

		// Calculate chunk hash
		chunkHash := sha256.Sum256(buffer[:n])
		chunkHashHex := hex.EncodeToString(chunkHash[:])

		metadata.Chunks = append(metadata.Chunks, ChunkInfo{
			Index: i,
			Hash:  chunkHashHex,
			Size:  int64(n),
		})
	}

	return metadata, nil
}

// SaveChunks saves file chunks to local storage
func SaveChunks(filePath string, metadata *ChunkMetadata) error {
	// Create chunks directory
	chunkDir := filepath.Join(ChunksDir, metadata.FileHash)
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		return err
	}

	// Open source file
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write each chunk
	buffer := make([]byte, ChunkSize)
	for i := 0; i < metadata.TotalChunks; i++ {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return err
		}

		// Write chunk file
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d.dat", i))
		if err := os.WriteFile(chunkPath, buffer[:n], 0644); err != nil {
			return err
		}
	}

	// Save metadata JSON
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	metadataPath := filepath.Join(chunkDir, "metadata.json")
	return os.WriteFile(metadataPath, metadataJSON, 0644)
}
