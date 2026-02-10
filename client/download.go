package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"p2p/common"
)

// FileInfo represents file metadata from tracker
type FileInfo struct {
	FileName    string      `json:"file_name"`
	FileHash    string      `json:"file_hash"`
	FileSize    int64       `json:"file_size"`
	ChunkSize   int64       `json:"chunk_size"`
	TotalChunks int         `json:"total_chunks"`
	Chunks      []ChunkInfo `json:"chunks"`
	Peers       []string    `json:"peers"`
}

// DownloadFile downloads a file from peers using P2P chunk transfer
func DownloadFile(groupID, fileName, destPath string) error {
	// 1. Get file info from tracker
	fileInfo, err := queryFileInfo(groupID, fileName)
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	if len(fileInfo.Peers) == 0 {
		return errors.New("no peers available for download")
	}

	fmt.Printf("File hash: %s...\n", fileInfo.FileHash[:16])
	fmt.Printf("Total chunks: %d\n", fileInfo.TotalChunks)
	fmt.Printf("Available peers: %d\n", len(fileInfo.Peers))

	// 2. Download chunks from peers
	chunks := make([][]byte, fileInfo.TotalChunks)

	for i := 0; i < fileInfo.TotalChunks; i++ {
		// Round-robin peer selection
		peer := fileInfo.Peers[i%len(fileInfo.Peers)]
		
		fmt.Printf("Downloading chunk %d/%d from %s...\n", i+1, fileInfo.TotalChunks, peer)
		
		chunkData, err := requestChunk(peer, fileInfo.FileHash, i)
		if err != nil {
			return fmt.Errorf("failed to download chunk %d: %v", i, err)
		}

		// Validate chunk hash
		if !validateChunkHash(chunkData, fileInfo.Chunks[i].Hash) {
			return fmt.Errorf("chunk %d hash mismatch", i)
		}

		chunks[i] = chunkData
	}

	fmt.Println("All chunks downloaded and validated ✓")

	// 3. Assemble file
	if err := assembleFile(chunks, destPath); err != nil {
		return fmt.Errorf("failed to assemble file: %v", err)
	}

	// 4. Save to local chunks directory
	metadata := &ChunkMetadata{
		FileName:    fileInfo.FileName,
		FileSize:    fileInfo.FileSize,
		FileHash:    fileInfo.FileHash,
		ChunkSize:   fileInfo.ChunkSize,
		TotalChunks: fileInfo.TotalChunks,
		Chunks:      fileInfo.Chunks,
	}
	
	if err := saveDownloadedChunks(chunks, metadata); err != nil {
		fmt.Printf("Warning: failed to save chunks locally: %v\n", err)
	}

	return nil
}

// queryFileInfo requests file metadata from tracker
func queryFileInfo(groupID, fileName string) (*FileInfo, error) {
	resp := SendToTracker(Message{
		Cmd:  "get_file_info",
		Args: []string{groupID, fileName},
	})

	if resp.Status != "ok" {
		return nil, fmt.Errorf("tracker error: %v", resp.Data)
	}

	// Parse response data
	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid response format")
	}

	jsonData, err := json.Marshal(dataMap)
	if err != nil {
		return nil, err
	}

	var fileInfo FileInfo
	if err := json.Unmarshal(jsonData, &fileInfo); err != nil {
		return nil, err
	}

	return &fileInfo, nil
}

// requestChunk requests a specific chunk from a peer
func requestChunk(peerAddr, fileHash string, chunkIdx int) ([]byte, error) {
	// Connect to peer
	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %v", err)
	}
	defer conn.Close()

	// Send handshake
	err = common.Send(conn, PeerRequest{
		Cmd:      "handshake",
		FileHash: fileHash,
	})
	if err != nil {
		return nil, err
	}

	var handshakeResp PeerResponse
	if err := common.Recv(conn, &handshakeResp); err != nil {
		return nil, err
	}

	if handshakeResp.Status != "ok" {
		return nil, errors.New("handshake failed")
	}

	// Close and reconnect for get_piece
	conn.Close()
	conn, err = net.Dial("tcp", peerAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Request chunk
	err = common.Send(conn, PeerRequest{
		Cmd:      "get_piece",
		FileHash: fileHash,
		PieceIdx: chunkIdx,
	})
	if err != nil {
		return nil, err
	}

	var pieceResp PeerResponse
	if err := common.Recv(conn, &pieceResp); err != nil {
		return nil, err
	}

	if pieceResp.Status != "ok" {
		return nil, errors.New("chunk download failed")
	}

	return pieceResp.Data, nil
}

// validateChunkHash verifies chunk data matches expected SHA256 hash
func validateChunkHash(data []byte, expectedHash string) bool {
	hash := sha256.Sum256(data)
	actualHash := hex.EncodeToString(hash[:])
	return actualHash == expectedHash
}

// assembleFile concatenates chunks and writes to destination
func assembleFile(chunks [][]byte, destPath string) error {
	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, chunk := range chunks {
		if _, err := file.Write(chunk); err != nil {
			return err
		}
	}

	return nil
}

// saveDownloadedChunks saves downloaded chunks to local .chunks directory
func saveDownloadedChunks(chunks [][]byte, metadata *ChunkMetadata) error {
	chunkDir := filepath.Join(ChunksDir, metadata.FileHash)
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		return err
	}

	// Write chunks
	for i, chunk := range chunks {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d.dat", i))
		if err := os.WriteFile(chunkPath, chunk, 0644); err != nil {
			return err
		}
	}

	// Write metadata
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	metadataPath := filepath.Join(chunkDir, "metadata.json")
	return os.WriteFile(metadataPath, metadataJSON, 0644)
}
