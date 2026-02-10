# P2P File Sharing System

A BitTorrent-like peer-to-peer file sharing system built in Go with tracker coordination and chunk-based transfers.

## Features

- 🔄 **P2P File Transfer** - Direct peer-to-peer chunk-based downloads
- 🔒 **SHA256 Validation** - Cryptographic hash verification per chunk
- 📡 **Tracker Coordination** - Centralized peer discovery
- ⚡ **Background Peer Servers** - Non-blocking daemon processes
- 👥 **Multi-user Support** - User authentication and session management
- 📁 **Group-based Sharing** - Organize files into groups
- ⚙️ **Configurable Tracker** - External config file support

## Quick Start

### 1. Build
```bash
go build -o tracker_bin ./tracker
go build -o client_bin ./client
```

### 2. Start Tracker
```bash
# Default (localhost:9000)
./tracker_bin

# Or with config file
./tracker_bin tracker_info.txt 1
```

### 3. Upload File (Alice)
```bash
mkdir alice && cd alice
../client_bin create_user Alice pass123
../client_bin login Alice pass123
../client_bin create_group photos
../client_bin upload_file ../myfile.jpg photos
```

### 4. Download File (Bob)
```bash
mkdir bob && cd bob
../client_bin create_user Bob password456
../client_bin login Bob password456
../client_bin download_file photos myfile.jpg downloaded.jpg
```

## Commands

### User Management
- `create_user <username> <password>` - Create account
- `login <username> <password>` - Login and start peer server
- `logout` - Logout and clear session
- `status` - Show current session info

### Group Management
- `create_group <groupID>` - Create file sharing group
- `list_groups` - Display all groups in network

### File Operations
- `upload_file <file> <groupID>` - Upload and share file
- `list_files <groupID>` - List files in group
- `download_file <groupID> <fileName> <destPath>` - Download from peers
- `show_downloads` - Display locally downloaded files
- `stop_sharing <groupID> <fileName>` - Stop sharing a file

## Architecture

```
┌─────────────┐
│   Tracker   │ ← Coordinates peers, stores metadata
│   :9000     │
└──────┬──────┘
       │
   ┌───┴───┐
   │       │
┌──▼──┐ ┌──▼──┐
│Alice│ │ Bob │ ← Peer servers transfer chunks
│:58827│ │:58841│
└─────┘ └─────┘
```

## Technical Details

- **Language**: Go
- **Protocol**: TCP with JSON messaging
- **Chunk Size**: 512KB
- **Hash Algorithm**: SHA256
- **Peer Discovery**: Centralized tracker
- **Transfer**: Sequential chunk download with validation

## Project Structure

```
P2P/
├── tracker/           # Tracker server
│   ├── main.go       # Entry point with config support
│   ├── server.go     # Connection handler
│   ├── handlers.go   # Command handlers
│   ├── protocol.go   # Message types
│   └── state.go      # In-memory state
├── client/           # P2P client
│   ├── main.go       # CLI entry point
│   ├── session.go    # Session management
│   ├── chunk.go      # Chunking logic
│   ├── download.go   # P2P download
│   └── peer_server.go # Background peer server
├── common/           # Shared utilities
│   └── net.go        # Network helpers
└── tracker_info.txt  # Tracker configuration
```

## License

Educational project - use freely!

## Acknowledgments

Built as a learning exercise to understand BitTorrent-like P2P systems.
