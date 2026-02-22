package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// ── buildRarityOrder unit tests ───────────────────────────────────────────────

// TestRarityOrder_AllPeersHaveAllChunks verifies that when every peer has
// every chunk the output is a stable sequential order (all counts equal).
func TestRarityOrder_AllPeersHaveAllChunks(t *testing.T) {
	bf := map[string][]bool{
		"peer1": {true, true, true, true},
		"peer2": {true, true, true, true},
		"peer3": {true, true, true, true},
	}
	got := buildRarityOrder(bf, 4)
	want := []int{0, 1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("equal-count order: want %v got %v", want, got)
	}
}

// TestRarityOrder_OnePeerHasOneChunk verifies that chunks held by fewer peers
// sort before chunks held by more peers.
func TestRarityOrder_OnePeerHasOneChunk(t *testing.T) {
	// Chunk 2 is held only by peer1 → rarest; chunks 0,1,3 held by both
	bf := map[string][]bool{
		"peer1": {true, true, true, true},
		"peer2": {true, true, false, true},
	}
	got := buildRarityOrder(bf, 4)
	// chunk 2 must be first (count=1); rest (count=2) in stable index order
	if got[0] != 2 {
		t.Errorf("expected chunk 2 first (rarest), got chunk %d first; full order: %v", got[0], got)
	}
	// The remaining three must be {0,1,3} in index order
	rest := got[1:]
	wantRest := []int{0, 1, 3}
	if !reflect.DeepEqual(rest, wantRest) {
		t.Errorf("remaining order: want %v got %v", wantRest, rest)
	}
}

// TestRarityOrder_MultipleRarities checks a full gradient of rarities.
func TestRarityOrder_MultipleRarities(t *testing.T) {
	// chunk 0: 3 peers, chunk 1: 2 peers, chunk 2: 1 peer, chunk 3: 0 peers
	bf := map[string][]bool{
		"peer1": {true, true, true, false},
		"peer2": {true, true, false, false},
		"peer3": {true, false, false, false},
	}
	got := buildRarityOrder(bf, 4)
	// Expected ascending count: chunk3(0) < chunk2(1) < chunk1(2) < chunk0(3)
	want := []int{3, 2, 1, 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("gradient order: want %v got %v", want, got)
	}
}

// TestRarityOrder_NilBitfieldMeansAllChunks verifies that a nil bitfield
// (returned by old peers that don't support get_bitfield) is treated as
// "peer has all chunks", which increases each chunk's count by 1.
func TestRarityOrder_NilBitfieldMeansAllChunks(t *testing.T) {
	// peer1 has all via nil, peer2 only has chunk 0 → chunk 0 has count 2, others count 1
	bf := map[string][]bool{
		"peer1": nil,
		"peer2": {true, false, false},
	}
	got := buildRarityOrder(bf, 3)
	// chunks 1,2 (count=1) should precede chunk 0 (count=2)
	if got[0] != 1 && got[0] != 2 {
		t.Errorf("expected chunk 1 or 2 first, got %d; full order: %v", got[0], got)
	}
	if got[1] != 1 && got[1] != 2 {
		t.Errorf("expected chunk 1 or 2 second, got %d; full order: %v", got[1], got)
	}
	if got[2] != 0 {
		t.Errorf("expected chunk 0 last (most common), got %d; full order: %v", got[2], got)
	}
}

// TestRarityOrder_TieBreakByIndex verifies that equal-rarity chunks are
// returned in ascending index order (stable sort).
func TestRarityOrder_TieBreakByIndex(t *testing.T) {
	// All chunks equally rare (1 peer each in a diagonal pattern, but same count)
	bf := map[string][]bool{
		"peer1": {true, true, true, true, true},
	}
	got := buildRarityOrder(bf, 5)
	want := []int{0, 1, 2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tie-break by index: want %v got %v", want, got)
	}
}

// TestRarityOrder_SingleChunk verifies the 1-chunk edge case.
func TestRarityOrder_SingleChunk(t *testing.T) {
	bf := map[string][]bool{
		"peer1": {true},
		"peer2": {true},
	}
	got := buildRarityOrder(bf, 1)
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("single chunk: want [0] got %v", got)
	}
}

// TestRarityOrder_EmptyPeerMap verifies that with no peers all chunks get
// count 0 and are returned in ascending order.
func TestRarityOrder_EmptyPeerMap(t *testing.T) {
	got := buildRarityOrder(map[string][]bool{}, 4)
	want := []int{0, 1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("empty peers: want %v got %v", want, got)
	}
}

// TestRarityOrder_ShorterBitfieldThanTotalChunks checks that a bitfield shorter
// than totalChunks (peer only knows about early chunks) works correctly.
// Chunks beyond the bitfield length are counted as 0 for that peer.
func TestRarityOrder_ShorterBitfieldThanTotalChunks(t *testing.T) {
	// peer1 bitfield only covers chunks 0-1; chunk 2 isn't covered → count 0
	bf := map[string][]bool{
		"peer1": {true, true}, // len=2, but totalChunks=3
	}
	got := buildRarityOrder(bf, 3)
	// chunk 2 (count=0) is rarest, then 0 and 1 (count=1 each)
	if got[0] != 2 {
		t.Errorf("expected chunk 2 first (count=0), got %d; full: %v", got[0], got)
	}
	if got[1] != 0 || got[2] != 1 {
		t.Errorf("expected [2,0,1] got %v", got)
	}
}

// ── Empty file rejection unit tests ──────────────────────────────────────────

// TestChunkFile_RejectsEmptyFile verifies that ChunkFile returns an error for a
// zero-byte file rather than producing metadata with 0 chunks.
func TestChunkFile_RejectsEmptyFile(t *testing.T) {
	tmp := t.TempDir()
	emptyPath := filepath.Join(tmp, "empty.bin")

	if err := os.WriteFile(emptyPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ChunkFile(emptyPath)
	if err == nil {
		t.Fatal("expected an error for empty file, got nil")
	}
	t.Logf("✓ ChunkFile correctly rejects empty file: %v", err)
}

// TestChunkFile_AcceptsNonEmptyFile verifies that a non-empty file is chunked
// without error and produces at least one chunk with a valid hash.
func TestChunkFile_AcceptsNonEmptyFile(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "data.bin")
	content := []byte("Hello, P2P world! This is a test file with real content.")

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	meta, err := ChunkFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.TotalChunks < 1 {
		t.Errorf("expected at least 1 chunk, got %d", meta.TotalChunks)
	}
	if meta.FileHash == "" {
		t.Error("FileHash must not be empty")
	}
	if meta.FileSize != int64(len(content)) {
		t.Errorf("FileSize: want %d got %d", len(content), meta.FileSize)
	}
	for i, c := range meta.Chunks {
		if c.Hash == "" {
			t.Errorf("chunk %d has empty hash", i)
		}
	}
	t.Logf("✓ ChunkFile accepted %d-byte file → %d chunk(s), hash=%s...",
		len(content), meta.TotalChunks, meta.FileHash[:16])
}

// TestChunkFile_MultiChunkFile verifies correct chunk count for a file that
// spans exactly 2 chunks.
func TestChunkFile_MultiChunkFile(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "twochunks.bin")

	// Write 1.5× ChunkSize bytes to force exactly 2 chunks
	size := ChunkSize + ChunkSize/2
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatal(err)
	}

	meta, err := ChunkFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.TotalChunks != 2 {
		t.Errorf("expected 2 chunks, got %d", meta.TotalChunks)
	}
	// First chunk should be full ChunkSize; second should be ChunkSize/2
	if meta.Chunks[0].Size != ChunkSize {
		t.Errorf("chunk 0 size: want %d got %d", ChunkSize, meta.Chunks[0].Size)
	}
	if meta.Chunks[1].Size != int64(ChunkSize/2) {
		t.Errorf("chunk 1 size: want %d got %d", ChunkSize/2, meta.Chunks[1].Size)
	}
	t.Logf("✓ 2-chunk file: sizes %d + %d bytes", meta.Chunks[0].Size, meta.Chunks[1].Size)
}

// ── validateChunkHash unit tests ──────────────────────────────────────────────

// TestValidateChunkHash_CorrectHash verifies matching hash returns true.
func TestValidateChunkHash_CorrectHash(t *testing.T) {
	data := []byte("test chunk data")
	meta, err := ChunkFile(func() string {
		tmp := t.TempDir()
		p := filepath.Join(tmp, "f.bin")
		os.WriteFile(p, data, 0644)
		return p
	}())
	if err != nil {
		t.Fatal(err)
	}
	if !validateChunkHash(data, meta.Chunks[0].Hash) {
		t.Error("expected true for matching hash, got false")
	}
}

// TestValidateChunkHash_WrongHash verifies mismatched hash returns false.
func TestValidateChunkHash_WrongHash(t *testing.T) {
	data := []byte("one thing")
	if validateChunkHash(data, "0000000000000000000000000000000000000000000000000000000000000000") {
		t.Error("expected false for bad hash, got true")
	}
}
