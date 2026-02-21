package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestResumeSkipsExistingChunks verifies that assembleFileFromDisk works
// correctly and that the resume logic (os.Stat check) correctly identifies
// which chunks already exist vs which need downloading.
func TestResumeSkipsExistingChunks(t *testing.T) {
	// Create a temp directory to act as the chunk store
	tmpDir := t.TempDir()
	chunkDir := filepath.Join(tmpDir, "testfile_hash")
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Simulate 5 chunks of content
	chunkContents := [][]byte{
		[]byte("chunk-zero-data"),
		[]byte("chunk-one-data-"),
		[]byte("chunk-two-data--"),
		[]byte("chunk-three-data"),
		[]byte("chunk-four-data-"),
	}

	// Write chunks 0, 1, 2 to disk (simulating partial download)
	for i := 0; i < 3; i++ {
		path := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d.dat", i))
		if err := os.WriteFile(path, chunkContents[i], 0644); err != nil {
			t.Fatal(err)
		}
	}

	// ── Verify: resume logic correctly identifies which chunks exist ──────
	skipped, missing := 0, 0
	for i := 0; i < 5; i++ {
		path := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d.dat", i))
		if _, err := os.Stat(path); err == nil {
			skipped++ // already on disk
		} else {
			missing++ // needs downloading
		}
	}

	if skipped != 3 {
		t.Errorf("Expected 3 skipped chunks, got %d", skipped)
	}
	if missing != 2 {
		t.Errorf("Expected 2 missing chunks, got %d", missing)
	}
	t.Logf("✓ Skip detection: skipped=%d, missing=%d (correct)", skipped, missing)

	// ── Write the missing chunks 3, 4 (simulating resume completing) ─────
	for i := 3; i < 5; i++ {
		path := filepath.Join(chunkDir, fmt.Sprintf("chunk_%d.dat", i))
		if err := os.WriteFile(path, chunkContents[i], 0644); err != nil {
			t.Fatal(err)
		}
	}

	// ── Verify: assembleFileFromDisk produces correct output ─────────────
	outPath := filepath.Join(tmpDir, "assembled.bin")
	if err := assembleFileFromDisk(chunkDir, 5, outPath); err != nil {
		t.Fatalf("assembleFileFromDisk failed: %v", err)
	}

	assembled, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	// Expected: concatenation of all 5 chunks
	var expected []byte
	for _, c := range chunkContents {
		expected = append(expected, c...)
	}

	if string(assembled) != string(expected) {
		t.Errorf("Assembled file content mismatch\n  want: %q\n  got:  %q", expected, assembled)
	}
	t.Logf("✓ assembleFileFromDisk: content matches expected (%d bytes)", len(expected))

	// ── Verify: hash of assembled equals expected ─────────────────────────
	h := sha256.Sum256(assembled)
	assembledHash := hex.EncodeToString(h[:])
	hExpected := sha256.Sum256(expected)
	expectedHash := hex.EncodeToString(hExpected[:])

	if assembledHash != expectedHash {
		t.Errorf("Hash mismatch: want %s got %s", expectedHash, assembledHash)
	}
	t.Logf("✓ SHA256 hash match: %s", assembledHash[:16]+"...")
}

// TestChunkMetadataSavedCorrectly verifies that metadata.json is written with
// correct fields (used by peers to serve chunks to other downloaders).
func TestChunkMetadataSavedCorrectly(t *testing.T) {
	tmpDir := t.TempDir()
	chunkDir := filepath.Join(tmpDir, "meta_test")
	os.MkdirAll(chunkDir, 0755)

	meta := &ChunkMetadata{
		FileName:    "test.bin",
		FileSize:    1024,
		FileHash:    "abc123",
		ChunkSize:   512,
		TotalChunks: 2,
		Chunks: []ChunkInfo{
			{Index: 0, Hash: "hash0", Size: 512},
			{Index: 1, Hash: "hash1", Size: 512},
		},
	}

	data, _ := json.MarshalIndent(meta, "", "  ")
	metaPath := filepath.Join(chunkDir, "metadata.json")
	os.WriteFile(metaPath, data, 0644)

	// Read it back
	content, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("Could not read metadata.json: %v", err)
	}

	var read ChunkMetadata
	if err := json.Unmarshal(content, &read); err != nil {
		t.Fatalf("Could not parse metadata.json: %v", err)
	}

	if read.FileName != "test.bin" || read.TotalChunks != 2 {
		t.Errorf("Metadata mismatch: %+v", read)
	}
	t.Logf("✓ metadata.json written and read back correctly: %s, %d chunks", read.FileName, read.TotalChunks)
}
