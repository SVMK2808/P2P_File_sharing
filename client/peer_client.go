package main

import (
	"net"
	"p2p/common"
)

func RequestHandshake(peerAddr string, fileHash string){
	// PLACEHOLDER
	// 
	// Future Responsibilities:
	// 1. connect to peerAddr
	// 2. send PeerRequest{Cmd : "handshake", FileHash : filehash}
	// 3. receive PeerResponse
	// 4. parse availability / bitfield 
	// 
	// No scheduling logic here
	// No retries here 
	// No tracker calls here

	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return
	}

	defer conn.Close()

	req := PeerRequest {
		Cmd:		"handshake",
		FileHash: 	fileHash,
	}

	common.Send(conn, req)

	var resp PeerResponse
	common.Recv(conn, &resp)

	// ignore response for now
}

func RequestPiece(peerAddr string, fileHash string, pieceIdx int){
	// PLACEHOLDER
	// 
	// Future Responsibilities:
	// 1. connect to peerAddr
	// 2. send get_piece request
	// 3. receive raw bytes
	// 4. write to correct offset on disk
	// 5. mark piece as owned
	// 
	// No hashing here.
	// No validation here.
	// No tracker interaction.

	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return 
	}
	defer conn.Close()

	req := PeerRequest{
		Cmd:		"get_piece",
		FileHash: 	fileHash,
		PieceIdx: 	pieceIdx,
	}

	common.Send(conn, req)

	var resp PeerResponse
	common.Recv(conn, &resp)
}
