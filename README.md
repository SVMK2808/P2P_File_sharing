# P2P File Sharing System with DHT

A distributed peer-to-peer file sharing system with tracker-to-tracker DHT synchronization and client chunk tracking for rarest-first downloads.

---

## Architecture

### Hybrid Design

**Trackers (Control Plane)**
- Form a DHT ring for metadata synchronization
- Store user accounts, groups, and file metadata
- Always-online for client bootstrapping
- DHT port = tracker port + 1000

**Clients (Data Plane)**
- Use trackers for user operations
- Direct peer-to-peer file transfers
- Chunk-based downloads with SHA256 verification
- Future: DHT chunk tracking for rarest-first

```
┌─────────────┐  DHT Sync   ┌─────────────┐  DHT Sync   ┌─────────────┐
│  Tracker 1  │◄────────────►│  Tracker 2  │◄────────────►│  Tracker 3  │
│  :9000      │   Gossip    │  :9001      │   Gossip    │  :9002      │
│  DHT:10000  │             │  DHT:10001  │             │  DHT:10002  │
└──────▲──────┘             └──────▲──────┘             └──────▲──────┘
       │                           │                           │
       │  Client Operations        │                           │
       │                           │                           │
    ┌──┴───────┐           ┌──────┴───┐              ┌────────┴──┐
    │ Client 1 │◄─────────►│ Client 2 │◄────────────►│ Client 3  │
    └──────────┘  P2P xfer └──────────┘  P2P xfer   └───────────┘
```

---

## Build

```bash
# Build everything
make all

# Or build individually
make tracker   # Builds tracker_bin
make client    # Builds client_bin
```

---

## Running the System

### 1. Start Trackers

Edit `tracker_info.txt` to configure tracker addresses:
```
127.0.0.1:9000
127.0.0.1:9001
127.0.0.1:9002
```

Start 3 trackers (in separate terminals):
```bash
# Terminal 1
./tracker_bin tracker_info.txt 1

# Terminal 2
./tracker_bin tracker_info.txt 2

# Terminal 3
./tracker_bin tracker_info.txt 3
```

**Expected output:**
```
Using tracker address from config: 127.0.0.1:9000
Tracker DHT initialized on port 10000
DHT initialized for tracker sync
Tracker listening on 127.0.0.1:9000
Type 'quit' to stop the tracker
```

### 2. Use Client

#### Option A: Direct Commands

```bash
# Create user
./client_bin create_user Alice pass123

# Login (starts peer server in background)
./client_bin login Alice pass123

# Create group
./client_bin create_group mygroup

# Upload file to group
./client_bin upload_file /path/to/file.txt mygroup

# List groups
./client_bin list_groups

# List files in group
./client_bin list_files mygroup

# Download file
./client_bin download_file mygroup file.txt [output_path]

# Check status
./client_bin status

# Show downloaded files
./client_bin show_downloads

# Logout
./client_bin logout
```

#### Option B: POSIX Shell Interface

Use the interactive shell for a better experience:

```bash
# Start the POSIX shell
./p2p_shell

# Or if p2p_shell isn't executable:
bash p2p_shell
```

**Shell commands:**
```bash
p2p> create_user Alice pass123
p2p> login Alice pass123
p2p> create_group testgroup
p2p> upload_file myfile.txt testgroup
p2p> list_groups
p2p> list_files testgroup
p2p> download_file testgroup myfile.txt
p2p> status
p2p> logout
p2p> quit
```

---

## Available Commands

### User Management
- `create_user <username> <password>` - Create new user account
- `login <username> <password>` - Login and start peer server
- `logout` - Logout and stop peer server
- `status` - Show login status and peer server info

### Group Management
- `create_group <groupID>` - Create new group (you become owner)
- `list_groups` - List all groups in network
- `join_group <groupID>` - Request to join group
- `accept_request <groupID> <username>` - Accept join request (owner only)
- `leave_group <groupID>` - Leave a group

### File Operations
- `upload_file <filepath> <groupID>` - Chunk and upload file to group
- `list_files <groupID>` - List files in group
- `download_file <groupID> <filename> [destpath]` - Download file
- `show_downloads` - Show downloaded files
- `stop_sharing <groupID> <filename>` - Stop sharing a file

---

## File Structure

```
P2P/
├── tracker/              # Tracker implementation
│   ├── main.go          # Tracker server
│   ├── handlers.go      # Request handlers
│   ├── dht_adapter.go   # DHT integration
│   ├── state.go         # State structures
│   └── persistence.go   # Disk persistence
├── client/              # Client implementation
│   ├── main.go          # Client entry point
│   ├── download.go      # Download logic
│   ├── upload.go        # Upload & chunking
│   ├── peer_server.go   # P2P server
│   └── tracker_conn.go  # Tracker communication
├── dht/                 # DHT package (DynamoDB-inspired)
│   ├── node.go          # Core DHT node
│   ├── gossip.go        # Gossip protocol
│   ├── consistent_hash.go # Consistent hashing
│   ├── vector_clock.go  # Conflict resolution
│   ├── merkle_tree.go   # Anti-entropy
│   └── p2p_client.go    # P2P-specific wrapper
├── common/              # Shared utilities
├── bin/                 # Helper shell scripts
├── tracker_bin          # Compiled tracker
├── client_bin           # Compiled client
├── p2p_shell            # POSIX shell interface
└── tracker_info.txt     # Tracker configuration
```

---

## Testing

### Basic Workflow Test

```bash
# Terminal 1: Start tracker
./tracker_bin tracker_info.txt 1

# Terminal 2: Alice
./client_bin create_user Alice pass123
./client_bin login Alice pass123
./client_bin create_group testgroup
echo "Hello from Alice" > alice.txt
./client_bin upload_file alice.txt testgroup

# Terminal 3: Bob
./client_bin create_user Bob pass456
./client_bin login Bob pass456
./client_bin list_groups          # Should see testgroup
./client_bin list_files testgroup # Should see alice.txt
./client_bin download_file testgroup alice.txt downloaded_alice.txt
cat downloaded_alice.txt           # Verify content
```

### Multi-Tracker Sync Test

```bash
# Start 3 trackers
./tracker_bin tracker_info.txt 1  # Terminal 1
./tracker_bin tracker_info.txt 2  # Terminal 2
./tracker_bin tracker_info.txt 3  # Terminal 3

# Create user on tracker 1
./client_bin create_user Alice pass123

# Login via tracker 2 (should work due to DHT sync)
# Verify user was synced across all trackers
```

---

## Features

✅ **Implemented:**
- Multi-tracker support with automatic failover
- Tracker DHT ring for metadata synchronization
- User accounts with password authentication
- Group-based file sharing
- File chunking (512KB chunks) with SHA256 verification
- Peer-to-peer chunk transfers
- Progress tracking for downloads
- Persistent state (survives tracker restarts)
- Session management (auto-restore on client restart)

🚧 **In Progress:**
- DHT-based chunk availability tracking
- Rarest-first download strategy
- Tracker handler DHT integration

---

## Environment

- **Language:** Go 1.x
- **Platform:** Linux/macOS
- **Dependencies:** 
  - BadgerDB (embedded key-value store for DHT)
  - Standard Go libraries

---

## Configuration

### Tracker Config (`tracker_info.txt`)
```
# One tracker address per line
127.0.0.1:9000
127.0.0.1:9001
127.0.0.1:9002
```

### DHT Ports
- Automatically set to tracker_port + 1000
- Example: Tracker :9000 → DHT :10000

---

## Troubleshooting

**"no trackers available"**
- Ensure tracker is running
- Check `tracker_info.txt` exists and has correct addresses

**"insufficient replicas for write quorum"** (DHT)
- Start more tracker nodes (need 2+ for quorum)
- Check DHT gossip connections

**Download fails**
- Verify file exists: `./client_bin list_files <groupID>`
- Ensure uploader is online with peer server running
- Check network connectivity

**Tracker won't start**
- Check port not already in use: `lsof -i :9000`
- Verify tracker_info.txt format

---

## Architecture Notes

### Why Hybrid (Tracker + DHT)?

**Trackers with DHT:**
- Always-online infrastructure
- Solves bootstrapping problem (first client can connect)
- Trackers sync state via DHT gossip
- Fault-tolerant metadata storage

**Client P2P:**
- Direct chunk transfers (no tracker involvement)
- Scalable data plane
- Future: DHT chunk tracking for rarest-first

### Chunk Verification

Every chunk has SHA256 hash:
1. Upload: Calculate hash per chunk
2. Download: Verify hash after receiving
3. Entire file: Final SHA256 verification

---

## License

MIT License

## Author

SVMK - Advanced Operating Systems Project
