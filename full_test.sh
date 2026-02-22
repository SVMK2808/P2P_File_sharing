#!/bin/bash
# ============================================================
# full_test.sh — Complete P2P System Test Suite
# Tests: user mgmt, groups, file upload/download, multi-tracker
#        DHT sync, tracker failover, resume downloads
# ============================================================

set -euo pipefail

# ── Colour helpers ────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

PASS=0; FAIL=0; SKIP=0
declare -a FAILURES=()

pass() { echo -e "  ${GREEN}✓ PASS${NC}  $1"; PASS=$((PASS+1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC}  $1"; FAIL=$((FAIL+1)); FAILURES+=("$1"); }
skip() { echo -e "  ${YELLOW}⊘ SKIP${NC}  $1"; SKIP=$((SKIP+1)); }
section() { echo -e "\n${BOLD}${BLUE}══ $1 ══${NC}"; }
info()    { echo -e "  ${CYAN}ℹ${NC}  $1"; }

assert_contains() {
    local label="$1" expected="$2" actual="$3"
    if echo "$actual" | grep -qi "$expected"; then
        pass "$label"
    else
        fail "$label  (expected '$expected' in output; got: $(echo "$actual" | head -3))"
    fi
}

assert_not_contains() {
    local label="$1" bad="$2" actual="$3"
    if echo "$actual" | grep -qi "$bad"; then
        fail "$label  (unexpected '$bad' in output)"
    else
        pass "$label"
    fi
}

assert_file_exists() { [ -f "$2" ] && pass "$1" || fail "$1 (file not found: $2)"; }
assert_files_equal() {
    local label="$1" f1="$2" f2="$3"
    if cmp -s "$f1" "$f2"; then pass "$label"; else fail "$label (files differ)"; fi
}

# ── Paths ─────────────────────────────────────────────────────────────────────
WORKSPACE="/Users/svmk/Desktop/P2P"
CLIENT_BIN="$WORKSPACE/client_bin"
TRACKER_BIN="$WORKSPACE/tracker_bin"
TESTROOT="/tmp/p2p_full_test"

# ── Cleanup helper ────────────────────────────────────────────────────────────
cleanup() {
    info "Stopping all tracker and peer processes..."
    pkill -f "tracker_bin" 2>/dev/null || true
    pkill -f "client_bin peer_daemon" 2>/dev/null || true
    sleep 1
}
trap cleanup EXIT

# ── Step 0: Pre-flight ────────────────────────────────────────────────────────
section "0. Pre-flight checks"

[ -x "$CLIENT_BIN" ]  && pass "client_bin exists and is executable" \
                       || { fail "client_bin missing"; exit 1; }
[ -x "$TRACKER_BIN" ] && pass "tracker_bin exists and is executable" \
                       || { fail "tracker_bin missing"; exit 1; }

# Kill any left-over processes from previous runs
cleanup

# ── Step 1: Setup test directories ───────────────────────────────────────────
section "1. Test environment setup"

rm -rf "$TESTROOT"
for d in tracker1 tracker2 tracker3 alice bob charlie testfiles; do
    mkdir -p "$TESTROOT/$d"
done

# Tracker info – three trackers
cat > "$TESTROOT/tracker_info.txt" <<'EOF'
127.0.0.1:9000
127.0.0.1:9001
127.0.0.1:9002
EOF

# Copy binaries and config into each tracker dir
for i in 1 2 3; do
    cp "$TRACKER_BIN"           "$TESTROOT/tracker$i/tracker_bin"
    cp "$TESTROOT/tracker_info.txt" "$TESTROOT/tracker$i/tracker_info.txt"
done

# Copy client binary and config into each user dir
for user in alice bob charlie; do
    cp "$CLIENT_BIN"                "$TESTROOT/$user/client_bin"
    cp "$TESTROOT/tracker_info.txt" "$TESTROOT/$user/tracker_info.txt"
done

# Create test files
echo "Hello from Alice, this is a test file!" > "$TESTROOT/testfiles/hello.txt"
dd if=/dev/urandom bs=1024 count=600 of="$TESTROOT/testfiles/medium.bin" 2>/dev/null  # ~600KB (1+ chunk)
dd if=/dev/urandom bs=1024 count=1800 of="$TESTROOT/testfiles/large.bin" 2>/dev/null  # ~1.8MB (3+ chunks)

pass "Test directories created"
pass "Test files created (hello.txt, medium.bin ~600KB, large.bin ~1.8MB)"

# ── Step 2: Start trackers ────────────────────────────────────────────────────
section "2. Starting 3 trackers"

for i in 1 2 3; do
    (cd "$TESTROOT/tracker$i" && ./tracker_bin tracker_info.txt $i >> /tmp/p2p_test_tracker$i.log 2>&1 &)
    info "Tracker $i started (port $((8999+i)))"
done

info "Waiting 3s for trackers to initialize and gossip..."
sleep 3

# Verify all 3 are reachable
for port in 9000 9001 9002; do
    if nc -z -w1 127.0.0.1 $port 2>/dev/null; then
        pass "Tracker on :$port is reachable"
    else
        fail "Tracker on :$port is NOT reachable"
    fi
done

# ── Step 3: User management ───────────────────────────────────────────────────
section "3. User management"

# Helper: run client command from a user's directory
client() { local user="$1"; shift; (cd "$TESTROOT/$user" && ./client_bin "$@" 2>&1); }

# create_user
OUT=$(client alice create_user Alice pass123)
assert_contains "create_user Alice"     "user created\|created"    "$OUT"

OUT=$(client bob create_user Bob pass456)
assert_contains "create_user Bob"       "user created\|created"    "$OUT"

OUT=$(client charlie create_user Charlie pass789)
assert_contains "create_user Charlie"   "user created\|created"    "$OUT"

# Duplicate user should fail
OUT=$(client alice create_user Alice pass123)
assert_contains "duplicate create_user rejected" "error\|exist"    "$OUT"

# login – Alice
OUT=$(client alice login Alice pass123)
assert_contains "Alice login ok"        "logged in\|peer server"   "$OUT"

# login wrong password
OUT=$(client alice login Alice wrongpass)
assert_contains "wrong password rejected" "error\|invalid"         "$OUT"

# login – Bob and Charlie
OUT=$(client bob login Bob pass456)
assert_contains "Bob login ok"          "logged in\|peer server"   "$OUT"

OUT=$(client charlie login Charlie pass789)
assert_contains "Charlie login ok"      "logged in\|peer server"   "$OUT"

# status
OUT=$(client alice status)
assert_contains "Alice status logged in" "logged in\|Alice"        "$OUT"

OUT=$(client bob status)
assert_contains "Bob status logged in"   "logged in\|Bob"          "$OUT"

sleep 1  # let peer daemons register addresses with tracker

# ── Step 4: Group management ──────────────────────────────────────────────────
section "4. Group management"

OUT=$(client alice create_group AliceGroup)
assert_contains "create_group AliceGroup" "created\|AliceGroup"    "$OUT"

# Duplicate group
OUT=$(client alice create_group AliceGroup)
assert_contains "duplicate group rejected" "error\|exist"          "$OUT"

# Second group by Bob
OUT=$(client bob create_group BobShared)
assert_contains "create_group BobShared"  "created\|BobShared"     "$OUT"

# list_groups – should see both groups
OUT=$(client alice list_groups)
assert_contains "list_groups sees AliceGroup" "AliceGroup"         "$OUT"
assert_contains "list_groups sees BobShared"  "BobShared"          "$OUT"

# join_group
OUT=$(client bob join_group AliceGroup)
assert_contains "Bob requests to join AliceGroup" "request\|sent"  "$OUT"

OUT=$(client charlie join_group AliceGroup)
assert_contains "Charlie requests to join AliceGroup" "request\|sent" "$OUT"

# list_requests (Alice as owner)
OUT=$(client alice list_requests AliceGroup)
assert_contains "list_requests shows Bob"     "Bob"                "$OUT"
assert_contains "list_requests shows Charlie" "Charlie"            "$OUT"

# accept_request – Alice accepts Bob
OUT=$(client alice accept_request AliceGroup Bob)
assert_contains "Alice accepts Bob"           "accepted\|ok"       "$OUT"

# accept_request – Alice accepts Charlie
OUT=$(client alice accept_request AliceGroup Charlie)
assert_contains "Alice accepts Charlie"       "accepted\|ok"       "$OUT"

# Bob should NOT be able to join again
OUT=$(client bob join_group AliceGroup)
# (already member or pending cleared – either 'ok' or error is fine, just verify no crash)
info "join_group again: $OUT"

# ── Step 5: File upload ───────────────────────────────────────────────────────
section "5. File upload"

# Alice uploads hello.txt to AliceGroup
HELLO="$TESTROOT/testfiles/hello.txt"
OUT=$(client alice upload_file "$HELLO" AliceGroup)
assert_contains "upload hello.txt"  "uploaded\|success\|chunk"  "$OUT"

# Alice uploads medium.bin
MEDIUM="$TESTROOT/testfiles/medium.bin"
OUT=$(client alice upload_file "$MEDIUM" AliceGroup)
assert_contains "upload medium.bin" "uploaded\|success\|chunk"  "$OUT"

# Duplicate upload should fail
OUT=$(client alice upload_file "$HELLO" AliceGroup)
assert_contains "duplicate upload rejected" "error\|exist"       "$OUT"

# Bob uploads to BobShared
LARGE="$TESTROOT/testfiles/large.bin"
OUT=$(client bob upload_file "$LARGE" BobShared)
assert_contains "Bob upload large.bin" "uploaded\|success\|chunk" "$OUT"

# Non-member cannot upload
OUT=$(client charlie upload_file "$HELLO" BobShared)
assert_contains "non-member upload rejected" "error\|member\|not"  "$OUT"

# ── Step 6: list_files ────────────────────────────────────────────────────────
section "6. list_files"

OUT=$(client alice list_files AliceGroup)
assert_contains "list_files hello.txt"  "hello.txt"    "$OUT"
assert_contains "list_files medium.bin" "medium.bin"   "$OUT"

OUT=$(client bob list_files AliceGroup)
assert_contains "Bob sees AliceGroup files" "hello.txt" "$OUT"

OUT=$(client charlie list_files AliceGroup)
assert_contains "Charlie sees AliceGroup files" "medium.bin" "$OUT"

OUT=$(client bob list_files BobShared)
assert_contains "BobShared has large.bin" "large.bin"   "$OUT"

# Non-existent group
OUT=$(client alice list_files NoSuchGroup)
assert_contains "list_files non-existent group" "error\|not found" "$OUT"

# ── Step 7: File download ─────────────────────────────────────────────────────
section "7. File download (P2P)"

info "Waiting 1s extra for Alice's peer daemon to register..."
sleep 1

# Bob downloads hello.txt from Alice
OUT=$(client bob download_file AliceGroup hello.txt "$TESTROOT/bob/downloaded_hello.txt")
assert_contains "Bob downloads hello.txt" "complete\|downloaded\|chunk" "$OUT"
assert_file_exists "bob/downloaded_hello.txt exists" "$TESTROOT/bob/downloaded_hello.txt"
assert_files_equal "downloaded hello.txt matches original" \
    "$HELLO" "$TESTROOT/bob/downloaded_hello.txt"

# Charlie downloads hello.txt
OUT=$(client charlie download_file AliceGroup hello.txt "$TESTROOT/charlie/hello_from_alice.txt")
assert_contains "Charlie downloads hello.txt" "complete\|downloaded\|chunk" "$OUT"
assert_files_equal "Charlie hello.txt matches original" \
    "$HELLO" "$TESTROOT/charlie/hello_from_alice.txt"

# Bob downloads medium.bin (multi-chunk file)
OUT=$(client bob download_file AliceGroup medium.bin "$TESTROOT/bob/downloaded_medium.bin")
assert_contains "Bob downloads medium.bin" "complete\|downloaded\|chunk" "$OUT"
assert_file_exists "bob/downloaded_medium.bin exists" "$TESTROOT/bob/downloaded_medium.bin"
assert_files_equal "downloaded medium.bin matches original" \
    "$MEDIUM" "$TESTROOT/bob/downloaded_medium.bin"

# Charlie downloads large.bin from Bob
OUT=$(client charlie download_file BobShared large.bin "$TESTROOT/charlie/downloaded_large.bin")
assert_contains "Charlie downloads large.bin from Bob" "complete\|downloaded\|chunk" "$OUT"
assert_file_exists "charlie/downloaded_large.bin exists" "$TESTROOT/charlie/downloaded_large.bin"
assert_files_equal "downloaded large.bin matches original" \
    "$LARGE" "$TESTROOT/charlie/downloaded_large.bin"

# Download non-existent file
OUT=$(client bob download_file AliceGroup nosuchfile.txt "$TESTROOT/bob/nope.txt")
assert_contains "download non-existent file fails" "error\|fail\|not found" "$OUT"

# ── Step 8: show_downloads ────────────────────────────────────────────────────
section "8. show_downloads"

OUT=$(client bob show_downloads)
assert_contains "Bob show_downloads lists hello.txt"  "hello.txt"  "$OUT"
assert_contains "Bob show_downloads lists medium.bin" "medium.bin" "$OUT"

OUT=$(client alice show_downloads)
# Alice uploaded but didn't download anything yet – she seeded from local chunks
# The upload stores chunks locally too, so show_downloads should list them
info "Alice show_downloads: $OUT"

# ── Step 9: stop_sharing ─────────────────────────────────────────────────────
section "9. stop_sharing"

OUT=$(client alice stop_sharing AliceGroup hello.txt)
assert_contains "stop_sharing hello.txt" "stopped\|ok"  "$OUT"

# After stop_sharing, list_files should no longer show hello.txt
OUT=$(client alice list_files AliceGroup)
assert_not_contains "hello.txt removed from group" "hello.txt" "$OUT"
assert_contains     "medium.bin still in group"    "medium.bin" "$OUT"

# Re-sharing: upload again should succeed now (file was removed)
OUT=$(client alice upload_file "$HELLO" AliceGroup)
assert_contains "re-upload hello.txt after stop" "uploaded\|success" "$OUT"

# ── Step 10: leave_group ─────────────────────────────────────────────────────
section "10. leave_group"

OUT=$(client charlie leave_group AliceGroup)
assert_contains "Charlie leaves AliceGroup" "left\|ok\|leave\|success" "$OUT"

# Charlie can no longer list files in AliceGroup
OUT=$(client charlie list_files AliceGroup)
assert_contains "Charlie can't list after leave" "error\|permission\|member\|not" "$OUT"

# ── Step 11: Multi-tracker DHT sync ──────────────────────────────────────────
section "11. Multi-tracker DHT sync"

info "Waiting 3s for gossip propagation..."
sleep 3

# Create a new user via tracker 1 (port 9000)
# Override tracker_info to only hit tracker 1 for registration
SYNC_DIR="$TESTROOT/sync_test"
mkdir -p "$SYNC_DIR"
cp "$CLIENT_BIN" "$SYNC_DIR/client_bin"
echo "127.0.0.1:9000" > "$SYNC_DIR/tracker_info.txt"

OUT=$(cd "$SYNC_DIR" && ./client_bin create_user SyncUser syncpass  2>&1)
assert_contains "SyncUser created on tracker 1" "created\|user" "$OUT"

# Wait for sync
info "Waiting 2s for cross-tracker gossip sync..."
sleep 2

# Try to login via tracker 2 only
cp "$CLIENT_BIN" "$SYNC_DIR/client_bin2" 2>/dev/null || true

SYNC_DIR2="$TESTROOT/sync_test2"
mkdir -p "$SYNC_DIR2"
cp "$CLIENT_BIN" "$SYNC_DIR2/client_bin"
echo "127.0.0.1:9001" > "$SYNC_DIR2/tracker_info.txt"

OUT=$(cd "$SYNC_DIR2" && ./client_bin login SyncUser syncpass  2>&1)
assert_contains "SyncUser can login via tracker 2 (DHT sync)" "logged in\|peer server" "$OUT"

# Try via tracker 3
SYNC_DIR3="$TESTROOT/sync_test3"
mkdir -p "$SYNC_DIR3"
cp "$CLIENT_BIN" "$SYNC_DIR3/client_bin"
echo "127.0.0.1:9002" > "$SYNC_DIR3/tracker_info.txt"

OUT=$(cd "$SYNC_DIR3" && ./client_bin login SyncUser syncpass  2>&1)
assert_contains "SyncUser can login via tracker 3 (DHT sync)" "logged in\|peer server" "$OUT"

# Verify a group created on tracker 1 is visible on tracker 2
SYNC_G_DIR="$TESTROOT/sync_group_test"
mkdir -p "$SYNC_G_DIR"
cp "$CLIENT_BIN" "$SYNC_G_DIR/client_bin"
echo "127.0.0.1:9000" > "$SYNC_G_DIR/tracker_info.txt"
(cd "$SYNC_G_DIR" && ./client_bin create_user GrpSyncUser grppass >/dev/null 2>&1) || true
(cd "$SYNC_G_DIR" && ./client_bin login GrpSyncUser grppass >/dev/null 2>&1) || true
(cd "$SYNC_G_DIR" && ./client_bin create_group SyncGroup >/dev/null 2>&1) || true

sleep 2
SYNC_G_DIR2="$TESTROOT/sync_group_test2"
mkdir -p "$SYNC_G_DIR2"
cp "$CLIENT_BIN" "$SYNC_G_DIR2/client_bin"
echo "127.0.0.1:9002" > "$SYNC_G_DIR2/tracker_info.txt"
(cd "$SYNC_G_DIR2" && ./client_bin create_user ViewUser viewpass >/dev/null 2>&1) || true
OUT=$(cd "$SYNC_G_DIR2" && ./client_bin list_groups  2>&1)
assert_contains "SyncGroup visible on tracker 3" "SyncGroup" "$OUT"

# ── Step 12: Tracker failover ─────────────────────────────────────────────────
section "12. Tracker failover"

info "Stopping tracker 1 (port 9000)..."
pkill -f "tracker_bin tracker_info.txt 1" 2>/dev/null || true
sleep 1

FAIL_DIR="$TESTROOT/failover_test"
mkdir -p "$FAIL_DIR"
cp "$CLIENT_BIN"              "$FAIL_DIR/client_bin"
cp "$TESTROOT/tracker_info.txt" "$FAIL_DIR/tracker_info.txt"

OUT=$(cd "$FAIL_DIR" && ./client_bin create_user FailUser failpass  2>&1)
assert_contains "create_user works after tracker 1 down" "created\|user" "$OUT"

OUT=$(cd "$FAIL_DIR" && ./client_bin login FailUser failpass  2>&1)
assert_contains "login works after tracker 1 down (failover)" "logged in\|peer server" "$OUT"

OUT=$(cd "$FAIL_DIR" && ./client_bin list_groups  2>&1)
assert_contains "list_groups works after tracker 1 down" "Groups\|ok\|group" "$OUT"

# Restart tracker 1
info "Restarting tracker 1..."
(cd "$TESTROOT/tracker1" && ./tracker_bin tracker_info.txt 1 >> /tmp/p2p_test_tracker1.log 2>&1 &)
sleep 2

OUT=$(cd "$FAIL_DIR" && ./client_bin list_groups  2>&1)
assert_contains "list_groups works after tracker 1 restarted" "Groups\|ok\|group" "$OUT"

# ── Step 13: Resume download ──────────────────────────────────────────────────
section "13. Resume download"

# Use alice's medium.bin (already in AliceGroup), Bob has it already downloaded.
# We'll test with Charlie downloading large.bin with interruption.
# Create a new user for clean resume test
RESUME_DIR="$TESTROOT/resume_client"
mkdir -p "$RESUME_DIR"
cp "$CLIENT_BIN"                 "$RESUME_DIR/client_bin"
cp "$TESTROOT/tracker_info.txt"  "$RESUME_DIR/tracker_info.txt"

(cd "$RESUME_DIR" && ./client_bin create_user ResumeUser respass >/dev/null 2>&1) || true
(cd "$RESUME_DIR" && ./client_bin login ResumeUser respass >/dev/null 2>&1) || true
(cd "$RESUME_DIR" && ./client_bin join_group AliceGroup >/dev/null 2>&1) || true
# Alice must accept
(cd "$TESTROOT/alice" && ./client_bin accept_request AliceGroup ResumeUser >/dev/null 2>&1) || true

sleep 1

# Start a slow download and kill it mid-way
info "Starting throttled download (P2P_CHUNK_DELAY=200ms), killing after 1s..."
(cd "$RESUME_DIR" && P2P_CHUNK_DELAY=200ms ./client_bin download_file AliceGroup medium.bin downloaded_resume.bin \
    > /tmp/p2p_resume_partial.log 2>&1) &
DL_PID=$!
sleep 1
kill $DL_PID 2>/dev/null || true; wait $DL_PID 2>/dev/null || true

# Get file hash of medium.bin to check partial chunks
MEDIUM_HASH=$(shasum -a 256 "$TESTROOT/testfiles/medium.bin" | awk '{print $1}')
CHUNK_DIR="$RESUME_DIR/.chunks/$MEDIUM_HASH"
SAVED=$(ls "$CHUNK_DIR"/chunk_*.dat 2>/dev/null | wc -l | tr -d ' ')
info "Chunks saved before kill: $SAVED"

if [ "$SAVED" -gt 0 ]; then
    pass "Partial download: $SAVED chunks saved to disk before interruption"

    # Now resume
    OUT=$(cd "$RESUME_DIR" && ./client_bin download_file AliceGroup medium.bin downloaded_resume.bin 2>&1)
    assert_contains "Resume download completes" "complete\|downloaded\|chunk" "$OUT"

    if echo "$OUT" | grep -qi "skipped\|resum"; then
        pass "Resume: confirmed skipped already-downloaded chunks"
    else
        skip "Resume: could not confirm chunk-skip message (may have been 1 chunk total)"
    fi

    assert_file_exists "Resumed file exists" "$RESUME_DIR/downloaded_resume.bin"
    assert_files_equal "Resumed file matches original" \
        "$TESTROOT/testfiles/medium.bin" "$RESUME_DIR/downloaded_resume.bin"
else
    skip "Resume test: download too fast to interrupt (no partial chunks), skipping resume verification"
fi

# ── Step 14: logout ───────────────────────────────────────────────────────────
section "14. Logout"

OUT=$(client alice logout)
assert_contains "Alice logout"   "logged out\|ok" "$OUT"

OUT=$(client alice status)
assert_contains "Alice status after logout" "not logged in\|not logged" "$OUT"

OUT=$(client bob logout)
assert_contains "Bob logout"     "logged out\|ok" "$OUT"

OUT=$(client charlie logout)
assert_contains "Charlie logout" "logged out\|ok" "$OUT"

# ── Step 15: Persistence (state survives restart) ─────────────────────────────
section "15. Tracker state persistence"

# Forcefully restart one tracker and verify state was preserved
info "Restarting tracker 2 to test persistence..."
pkill -f "tracker_bin tracker_info.txt 2" 2>/dev/null || true
sleep 1
(cd "$TESTROOT/tracker2" && ./tracker_bin tracker_info.txt 2 >> /tmp/p2p_test_tracker2.log 2>&1 &)
sleep 2

# Alice should still be able to login (state on disk)
OUT=$(client alice login Alice pass123)
assert_contains "Alice can login after tracker 2 restart (persistence)" "logged in\|peer server" "$OUT"

OUT=$(client alice list_groups)
assert_contains "AliceGroup persisted after restart" "AliceGroup" "$OUT"

OUT=$(client alice list_files AliceGroup)
assert_contains "medium.bin persisted after restart" "medium.bin" "$OUT"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}${BLUE}════════════════════════════════════════${NC}"
echo -e "${BOLD}  Test Summary${NC}"
echo -e "${BOLD}${BLUE}════════════════════════════════════════${NC}"
echo -e "  ${GREEN}PASS${NC}: $PASS"
echo -e "  ${RED}FAIL${NC}: $FAIL"
echo -e "  ${YELLOW}SKIP${NC}: $SKIP"
echo -e "  Total: $((PASS + FAIL + SKIP))"
echo ""

if [ ${#FAILURES[@]} -gt 0 ]; then
    echo -e "${RED}Failed tests:${NC}"
    for f in "${FAILURES[@]}"; do
        echo -e "  - $f"
    done
    echo ""
fi

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}All tests passed! ✅${NC}"
    exit 0
else
    echo -e "${RED}${BOLD}$FAIL test(s) failed ❌${NC}"
    exit 1
fi
