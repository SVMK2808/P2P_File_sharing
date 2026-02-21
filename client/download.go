package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"p2p/common"
	"path/filepath"
	"sort"
	"time"
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

// DownloadFile downloads a file from peers using P2P chunk transfer (rarest-first).
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

	// 2. Query each peer for their bitfield → rarity map
	peerBitfields := getBitfields(fileInfo.Peers, fileInfo.FileHash)
	order := buildRarityOrder(peerBitfields, fileInfo.TotalChunks)
	fmt.Printf("Piece selection: rarest-first (queried %d peers)\n", len(peerBitfields))

	// 3. Download in rarest-first order
	chunks := make([][]byte, fileInfo.TotalChunks)

	for _, i := range order {
		// Only pick peers that have this specific chunk
		qualified := make([]string, 0)
		for peer, bf := range peerBitfields {
			// nil bf = old peer that doesn't support get_bitfield; assume it has all chunks
			if bf == nil || (i < len(bf) && bf[i]) {
				qualified = append(qualified, peer)
			}
		}
		if len(qualified) == 0 {
			qualified = fileInfo.Peers
		}

		peer := qualified[i%len(qualified)]
		fmt.Printf("Downloading chunk %d/%d from %s (rarest-first)...\n", i+1, fileInfo.TotalChunks, peer)

		chunkData, err := requestChunk(peer, fileInfo.FileHash, i)
		if err != nil {
			return fmt.Errorf("failed to download chunk %d: %v", i, err)
		}

		if !validateChunkHash(chunkData, fileInfo.Chunks[i].Hash) {
			return fmt.Errorf("chunk %d hash mismatch", i)
		}

		chunks[i] = chunkData
	}

	fmt.Println("All chunks downloaded and validated ✓")

	// 4. Assemble file
	if err := assembleFile(chunks, destPath); err != nil {
		return fmt.Errorf("failed to assemble file: %v", err)
	}

	// 5. Save to local chunks dir so we can seed
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

// getBitfields queries all peers for their bitfield (which chunks they have).
// Returns map[peerAddr][]bool where index = chunk index.
func getBitfields(peers []string, fileHash string) map[string][]bool {
	result := make(map[string][]bool)
	for _, peer := range peers {
		bf := queryBitfield(peer, fileHash)
		if bf != nil {
			result[peer] = bf
		}
	}
	// If no bitfields returned (old peers don't support get_bitfield), fall back
	if len(result) == 0 {
		for _, peer := range peers {
			result[peer] = nil // nil = assume has all chunks
		}
	}
	return result
}

// queryBitfield connects to a peer and requests its bitfield for fileHash.
func queryBitfield(peerAddr, fileHash string) []bool {
	conn, err := net.DialTimeout("tcp", peerAddr, 2*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	if err := common.Send(conn, PeerRequest{Cmd: "get_bitfield", FileHash: fileHash}); err != nil {
		return nil
	}

	var resp PeerResponse
	if err := common.Recv(conn, &resp); err != nil || resp.Status != "ok" || len(resp.Bitfield) == 0 {
		return nil
	}

	// Convert []int index list to []bool indexed by chunk index
	maxIdx := 0
	for _, idx := range resp.Bitfield {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	bf := make([]bool, maxIdx+1)
	for _, idx := range resp.Bitfield {
		bf[idx] = true
	}
	return bf
}

// buildRarityOrder returns chunk indices sorted by ascending peer availability (rarest first).
func buildRarityOrder(peerBitfields map[string][]bool, totalChunks int) []int {
	// Count how many peers have each chunk
	count := make([]int, totalChunks)
	for _, bf := range peerBitfields {
		if bf == nil {
			// Peer with unknown bitfield: assume it has everything
			for i := range count {
				count[i]++
			}
			continue
		}
		for i := 0; i < totalChunks; i++ {
			if i < len(bf) && bf[i] {
				count[i]++
			}
		}
	}

	// Sort chunk indices by count ascending (rarest first), then by index for stability
	indices := make([]int, totalChunks)
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		if count[indices[a]] != count[indices[b]] {
			return count[indices[a]] < count[indices[b]]
		}
		return indices[a] < indices[b]
	})
	return indices
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
