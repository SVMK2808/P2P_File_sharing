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

// DownloadFile downloads a file from peers using P2P chunk transfer.
// Resumable: already-downloaded chunks are skipped on restart.
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

	// 2. Prepare local chunk directory (supports resume + final assembly)
	chunkDir := filepath.Join(ChunksDir, fileInfo.FileHash)
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		return fmt.Errorf("failed to create chunk dir: %v", err)
	}

	// 3. Choose chunk download order: rarest-first or sequential (round-robin)
	var order []int
	var peerBitfields map[string][]bool // non-nil only in rarest-first mode

	if os.Getenv("P2P_RAREST_FIRST") != "" {
		peerBitfields = getBitfields(fileInfo.Peers, fileInfo.FileHash)
		order = buildRarityOrder(peerBitfields, fileInfo.TotalChunks)
		fmt.Printf("Piece selection: rarest-first (queried %d peers)\n", len(peerBitfields))
	} else {
		order = make([]int, fileInfo.TotalChunks)
		for i := range order {
			order[i] = i
		}
	}

	// 4. Download missing chunks in chosen order — skip those already on disk
	downloaded := 0
	skipped := 0
	for _, i := range order {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d.dat", i))

		// Resume: chunk already downloaded in a previous run
		if _, err := os.Stat(chunkPath); err == nil {
			skipped++
			continue
		}

		// Pick best peer for this chunk
		var peer string
		if peerBitfields != nil {
			// Rarest-first: prefer peers known to have this specific chunk
			qualified := make([]string, 0, len(peerBitfields))
			for p, bf := range peerBitfields {
				if bf == nil || (i < len(bf) && bf[i]) {
					qualified = append(qualified, p)
				}
			}
			if len(qualified) > 0 {
				peer = qualified[i%len(qualified)]
			} else {
				peer = fileInfo.Peers[i%len(fileInfo.Peers)]
			}
			fmt.Printf("Downloading chunk %d/%d from %s (rarest-first)...\n", i+1, fileInfo.TotalChunks, peer)
		} else {
			peer = fileInfo.Peers[i%len(fileInfo.Peers)]
			fmt.Printf("Downloading chunk %d/%d from %s...\n", i+1, fileInfo.TotalChunks, peer)
		}

		chunkData, err := requestChunk(peer, fileInfo.FileHash, i)
		if err != nil {
			return fmt.Errorf("failed to download chunk %d: %v", i, err)
		}

		if !validateChunkHash(chunkData, fileInfo.Chunks[i].Hash) {
			return fmt.Errorf("chunk %d hash mismatch", i)
		}

		// Write chunk immediately to disk (makes resume possible on interruption)
		if err := os.WriteFile(chunkPath, chunkData, 0644); err != nil {
			return fmt.Errorf("failed to save chunk %d: %v", i, err)
		}
		downloaded++

		// Testing: P2P_CHUNK_DELAY=500ms slows download so interruption can be triggered
		if d := os.Getenv("P2P_CHUNK_DELAY"); d != "" {
			if delay, err := time.ParseDuration(d); err == nil {
				time.Sleep(delay)
			}
		}
	} // end for-loop

	if skipped > 0 {
		fmt.Printf("Resumed: skipped %d already-downloaded chunks\n", skipped)
	}
	fmt.Printf("Downloaded %d new chunks. All chunks validated ✓\n", downloaded)

	// 4. Assemble file from disk chunks
	if err := assembleFileFromDisk(chunkDir, fileInfo.TotalChunks, destPath); err != nil {
		return fmt.Errorf("failed to assemble file: %v", err)
	}

	// 5. Save metadata for peer serving
	metadata := &ChunkMetadata{
		FileName:    fileInfo.FileName,
		FileSize:    fileInfo.FileSize,
		FileHash:    fileInfo.FileHash,
		ChunkSize:   fileInfo.ChunkSize,
		TotalChunks: fileInfo.TotalChunks,
		Chunks:      fileInfo.Chunks,
	}
	metadataJSON, _ := json.MarshalIndent(metadata, "", "  ")
	os.WriteFile(filepath.Join(chunkDir, "metadata.json"), metadataJSON, 0644)

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

// queryFileInfo requests file metadata from tracker.
// State.UserID is included so the tracker can enforce group membership.
func queryFileInfo(groupID, fileName string) (*FileInfo, error) {
	resp := SendToTracker(Message{
		Cmd:  "get_file_info",
		Args: []string{groupID, fileName, State.UserID},
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

// assembleFile concatenates chunks and writes to destination (used by upload verification)
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

// assembleFileFromDisk reads chunk files from disk in order and writes them to destPath.
// Used by resumable DownloadFile — chunks are already on disk from prior download steps.
func assembleFileFromDisk(chunkDir string, totalChunks int, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d.dat", i))
		data, err := os.ReadFile(chunkPath)
		if err != nil {
			return fmt.Errorf("missing chunk %d: %v", i, err)
		}
		if _, err := out.Write(data); err != nil {
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
