package main

import (
	"fmt"
	"net"
	"os"
	"p2p/common"
	"path/filepath"
)

// StartPeerServerWithListener creates a listener and returns it along with the actual address
func StartPeerServerWithListener(addr string) (net.Listener, string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return nil, ""
	}
	
	// Get the actual address (important for ":0" dynamic port assignment)
	actualAddr := ln.Addr().String()
	// Extract just the port part (e.g., "[::]:50123" -> ":50123")
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		actualAddr = fmt.Sprintf(":%d", tcpAddr.Port)
	}
	
	fmt.Printf("Peer server listening on %s\n", actualAddr)
	return ln, actualAddr
}

// AcceptPeerConnections accepts incoming peer connections (runs in goroutine)
func AcceptPeerConnections(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handlePeerConn(conn)
	}
}

// StartPeerServer is the original function kept for backward compatibility
func StartPeerServer(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handlePeerConn(conn)
	}
}

type PeerRequest struct {
	Cmd			string `json:"cmd"`
	FileHash	string `json:"file_hash"`
	PieceIdx	int `json:"piece_idx"`
}

type PeerResponse struct {
	Status  string `json:"status"`
	Data    []byte `json:"data,omitempty"`
	Bitfield []int `json:"bitfield,omitempty"` // Chunk indices this peer has
}

func handleHandshake(conn net.Conn, req PeerRequest){
	fileHash := req.FileHash
	
	// Check if we have this file
	chunkDir := filepath.Join(ChunksDir, fileHash)
	if _, err := os.Stat(chunkDir); os.IsNotExist(err) {
		common.Send(conn, PeerResponse{
			Status: "error",
		})
		return
	}
	
	common.Send(conn, PeerResponse{
		Status: "ok",
	})
}

func handleGetPiece(conn net.Conn, req PeerRequest){
	fileHash := req.FileHash
	chunkIdx := req.PieceIdx

	// Read chunk file
	chunkPath := filepath.Join(ChunksDir, fileHash, fmt.Sprintf("chunk_%d.dat", chunkIdx))
	data, err := os.ReadFile(chunkPath)
	if err != nil {
		common.Send(conn, PeerResponse{Status: "error"})
		return
	}

	common.Send(conn, PeerResponse{Status: "ok", Data: data})
}

// handleGetBitfield returns the set of chunk indices this peer has for a given file hash.
func handleGetBitfield(conn net.Conn, req PeerRequest) {
	chunkDir := filepath.Join(ChunksDir, req.FileHash)
	entries, err := os.ReadDir(chunkDir)
	if err != nil {
		common.Send(conn, PeerResponse{Status: "error"})
		return
	}

	bf := make([]int, 0)
	for _, e := range entries {
		var idx int
		if _, err := fmt.Sscanf(e.Name(), "chunk_%d.dat", &idx); err == nil {
			bf = append(bf, idx)
		}
	}
	common.Send(conn, PeerResponse{Status: "ok", Bitfield: bf})
}

func handlePeerConn(conn net.Conn){
	defer conn.Close()

	var req PeerRequest
	if err := common.Recv(conn, &req); err != nil {
		return 
	}

	switch req.Cmd {
	case "handshake":
		handleHandshake(conn, req)
	case "get_piece":
		handleGetPiece(conn, req)
	case "get_bitfield":
		handleGetBitfield(conn, req)
	default:
		common.Send(conn, PeerResponse{Status: "error"})
	}
}
