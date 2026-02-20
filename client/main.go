package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	// Load session at startup to restore login state
	LoadSession()
	
	// Load tracker configuration
	LoadTrackerConfig("tracker_info.txt")
	
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "create_user":
		resp := SendToTracker(Message{
			Cmd:  "create_user",
			Args: args,
		})

		fmt.Println(resp)

	case "login":
		// args[0] = username, args[1] = password
		State.UserID = args[0]
		
		resp := SendToTracker(Message{
			Cmd:  "login",
			Args: []string{args[0], args[1], ""},  // Address will be set by daemon
		})

		if resp.Status != "ok" {
			fmt.Println(resp)
			return
		}
		
		// Spawn background peer server daemon
		cmd := exec.Command(os.Args[0], "peer_daemon")
		cmd.Stdout = nil
		cmd.Stderr = nil
		
		if err := cmd.Start(); err != nil {
			fmt.Printf("Error starting peer server: %v\n", err)
			return
		}
		
		// Save session
		if err := SaveSession(); err != nil {
			fmt.Printf("Warning: Failed to save session: %v\n", err)
		}
		
		fmt.Println(resp)
		fmt.Printf("Peer server started in background (PID: %d)\n", cmd.Process.Pid)
		fmt.Println("You can now run other commands.")

	case "create_group":
		resp := SendToTracker(Message{
			Cmd:  "create_group",
			Args: []string{args[0], State.UserID},
		})
		
		if resp.Status == "ok" {
			if data, ok := resp.Data.(map[string]interface{}); ok {
				fmt.Printf("✓ Group '%s' created successfully\n", data["group_id"])
				fmt.Printf("  Owner: %s\n", data["owner"])
			} else {
				fmt.Println(resp)
			}
		} else {
			fmt.Println(resp)
		}

	case "upload_file":
		//args: [filePath, groupID]
		filePath := args[0]
		groupID := args[1]

		// 1. Chunk the file
		fmt.Println("Chunking file...")
		metadata, err := ChunkFile(filePath)
		if err != nil {
			fmt.Printf("Error chunking file: %v\n", err)
			return
		}

		// 2. Save chunks locally
		fmt.Println("Saving chunks...")
		err = SaveChunks(filePath, metadata)
		if err != nil {
			fmt.Printf("Error saving chunks: %v\n", err)
			return
		}

		// 3. Convert chunks to JSON
		chunksJSON, err := json.Marshal(metadata.Chunks)
		if err != nil {
			fmt.Printf("Error marshaling chunks: %v\n", err)
			return
		}

		// 4. Send to tracker
		resp := SendToTracker(Message{
			Cmd: "upload_file",
			Args: []string{
				metadata.FileName,
				groupID,
				State.UserID,
				fmt.Sprintf("%d", metadata.FileSize),
				metadata.FileHash,
				string(chunksJSON),
			},
		})

		if resp.Status == "ok" {
			if data, ok := resp.Data.(map[string]interface{}); ok {
				fmt.Printf("✓ File chunked and uploaded successfully\n")
				fmt.Printf("  File: %s\n", data["file_name"])
				fmt.Printf("  Group: %s\n", data["group_id"])
				fmt.Printf("  Size: %v bytes\n", data["file_size"])
				if fileHash, ok := data["file_hash"].(string); ok {
					fmt.Printf("  Hash: %s...\n", fileHash[:16])
				}
				if totalChunks, ok := data["total_chunks"].(float64); ok {
					fmt.Printf("  Chunks: %.0f\n", totalChunks)
				}
				fmt.Printf("  Chunks stored in: .chunks/%s/\n", metadata.FileHash)
			} else {
				fmt.Println(resp)
			}
		} else {
			fmt.Println(resp)
		}

	case "list_files":
		resp := SendToTracker(Message{
			Cmd:  "list_files",
			Args: []string{args[0]},
		})
		
		if resp.Status == "ok" {
			if fileList, ok := resp.Data.([]interface{}); ok {
				if len(fileList) == 0 {
					fmt.Printf("No files in group '%s'\n", args[0])
				} else {
					fmt.Printf("Files in group '%s':\n", args[0])
					fmt.Println("──────────────────────────────────────────────────────")
					for i, item := range fileList {
						if file, ok := item.(map[string]interface{}); ok {
							fmt.Printf("%d. %s\n", i+1, file["file_name"])
							fmt.Printf("   Size: %v bytes\n", file["file_size"])
							fmt.Printf("   Uploader: %s\n", file["uploader"])
							if i < len(fileList)-1 {
								fmt.Println()
							}
						}
					}
					fmt.Println("──────────────────────────────────────────────────────")
				}
			} else {
				fmt.Println(resp)
			}
		} else {
			fmt.Println(resp)
		}

	case "download_file":
		// args: [groupID, fileName, destPath (optional)]
		if len(args) < 2 {
			fmt.Println("Usage: download_file <groupID> <fileName> [destPath]")
			return
		}

		groupID := args[0]
		fileName := args[1]
		destPath := fileName
		if len(args) >= 3 {
			destPath = args[2]
		}

		fmt.Printf("Downloading '%s' from group '%s'...\n", fileName, groupID)

		err := DownloadFile(groupID, fileName, destPath)
		if err != nil {
			fmt.Printf("✗ Download failed: %v\n", err)
			return
		}

		fmt.Printf("✓ Download complete: %s\n", destPath)

	case "status":
		if State.UserID == "" {
			fmt.Println("Status: Not logged in")
			fmt.Println("Run './client_bin login <username> <password>' to login")
		} else {
			fmt.Println("Status: Logged in")
			fmt.Printf("User: %s\n", State.UserID)
			if State.ListenAddr != "" {
				fmt.Printf("Peer server: 127.0.0.1%s\n", State.ListenAddr)
			} else {
				fmt.Println("Peer server: Starting...")
			}
		}

	case "show_downloads":
		// Display downloaded files from .chunks directory
		entries, err := os.ReadDir(ChunksDir)
		if err != nil || len(entries) == 0 {
			fmt.Println("No downloaded files found")
			return
		}

		fmt.Println("Downloaded files:")
		fmt.Println("─────────────────────────────────────────────")
		
		count := 0
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			
			// Read metadata.json
			metadataPath := filepath.Join(ChunksDir, entry.Name(), "metadata.json")
			data, err := os.ReadFile(metadataPath)
			if err != nil {
				continue
			}
			
			var metadata ChunkMetadata
			if err := json.Unmarshal(data, &metadata); err != nil {
				continue
			}
			
			count++
			fmt.Printf("%d. %s\n", count, metadata.FileName)
			fmt.Printf("   Size: %.2f MB\n", float64(metadata.FileSize)/(1024*1024))
			fmt.Printf("   Hash: %s...\n", metadata.FileHash[:16])
			fmt.Printf("   Chunks: %d\n", metadata.TotalChunks)
			if count < len(entries)-1 {
				fmt.Println()
			}
		}
		fmt.Println("─────────────────────────────────────────────")

	case "list_groups":
		resp := SendToTracker(Message{
			Cmd:  "list_groups",
			Args: []string{},
		})

		if resp.Status == "ok" {
			if msg, ok := resp.Data.(string); ok {
				fmt.Println(msg)
			} else if groupList, ok := resp.Data.([]interface{}); ok {
				fmt.Println("Groups in network:")
				fmt.Println("─────────────────────────────────────")
				for i, group := range groupList {
					if groupStr, ok := group.(string); ok {
						fmt.Printf("%d. %s\n", i+1, groupStr)
					}
				}
				fmt.Println("─────────────────────────────────────")
			}
		} else {
			fmt.Println(resp)
		}

	case "stop_sharing":
		// args: [groupID, fileName]
		if len(args) < 2 {
			fmt.Println("Usage: stop_sharing <groupID> <fileName>")
			return
		}

		if State.UserID == "" {
			fmt.Println("Error: Not logged in")
			return
		}

		groupID := args[0]
		fileName := args[1]

		resp := SendToTracker(Message{
			Cmd:  "stop_sharing",
			Args: []string{groupID, fileName, State.UserID},
		})

		if resp.Status == "ok" {
			fmt.Printf("✓ Stopped sharing '%s' in group '%s'\n", fileName, groupID)
			fmt.Println("Note: Local chunks are preserved (delete .chunks/<hash>/ manually if needed)")
		} else {
			fmt.Println(resp)
		}

	case "logout":
		if err := ClearSession(); err != nil {
			fmt.Printf("Error clearing session: %v\n", err)
			return
		}
		
		// Reset state
		State.UserID = ""
		State.ListenAddr = ""
		
		fmt.Println("✓ Logged out successfully")

	case "peer_daemon":
		// Hidden command - runs peer server in background
		// Load session to get UserID
		if State.UserID == "" {
			fmt.Println("Error: No active session")
			return
		}
		
		// Start peer server
		ln, actualAddr := StartPeerServerWithListener(":0")
		if ln == nil {
			fmt.Println("Error: Failed to start peer server")
			return
		}
		
		State.ListenAddr = actualAddr
		
		// Update tracker with actual address
		SendToTracker(Message{
			Cmd:  "update_address",
			Args: []string{State.UserID, "127.0.0.1" + actualAddr},
		})
		
		// Save updated session with address
		SaveSession()
		
		// Run peer server forever
		AcceptPeerConnections(ln)


	case "join_group":
		// args: [groupID]
		if len(args) < 1 {
			fmt.Println("Usage: join_group <groupID>")
			return
		}
		if State.UserID == "" {
			fmt.Println("Error: Not logged in")
			return
		}
		resp := SendToTracker(Message{
			Cmd:  "join_group",
			Args: []string{args[0], State.UserID},
		})
		if resp.Status == "ok" {
			fmt.Printf("✓ Join request sent to group '%s'\n", args[0])
			fmt.Println("Wait for group owner to accept your request.")
		} else {
			fmt.Println(resp)
		}

	case "list_requests":
		// args: [groupID]  — only group owner can list
		if len(args) < 1 {
			fmt.Println("Usage: list_requests <groupID>")
			return
		}
		if State.UserID == "" {
			fmt.Println("Error: Not logged in")
			return
		}
		resp := SendToTracker(Message{
			Cmd:  "list_requests",
			Args: []string{args[0], State.UserID},
		})
		if resp.Status == "ok" {
			if requests, ok := resp.Data.([]interface{}); ok {
				if len(requests) == 0 {
					fmt.Println("No pending requests")
				} else {
					fmt.Printf("Pending join requests for '%s':\n", args[0])
					fmt.Println("──────────────────────────")
					for i, r := range requests {
						fmt.Printf("%d. %v\n", i+1, r)
					}
					fmt.Println("──────────────────────────")
				}
			} else {
				fmt.Println(resp.Data)
			}
		} else {
			fmt.Println(resp)
		}

	case "accept_request":
		// args: [groupID, userID]
		if len(args) < 2 {
			fmt.Println("Usage: accept_request <groupID> <userID>")
			return
		}
		if State.UserID == "" {
			fmt.Println("Error: Not logged in")
			return
		}
		resp := SendToTracker(Message{
			Cmd:  "accept_requests",
			Args: []string{args[0], State.UserID, args[1]},
		})
		if resp.Status == "ok" {
			fmt.Printf("✓ Accepted '%s' into group '%s'\n", args[1], args[0])
		} else {
			fmt.Println(resp)
		}

	default:
		fmt.Printf("{error unknown command: %s}\n", cmd)
	}

}
