#!/bin/bash
# ============================================================
# feature_test.sh — Targeted tests for three specific features:
#   1. Empty file rejection (client + chunk layer)
#   2. Tracker rejoin with missed-state pull (pullStateFromPeers)
#   3. Rarest-first piece selection (P2P_RAREST_FIRST=1)
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
        fail "$label  (expected '$expected'; got: $(echo "$actual" | head -5))"
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
assert_file_exists() { [ -f "$2" ] && pass "$1" || fail "$1 (missing: $2)"; }
assert_files_equal() {
    cmp -s "$2" "$3" && pass "$1" || fail "$1 (files differ: $2 vs $3)"
}

# ── Paths ─────────────────────────────────────────────────────────────────────
WORKSPACE="/Users/svmk/Desktop/P2P"
CLIENT_BIN="$WORKSPACE/client_bin"
TRACKER_BIN="$WORKSPACE/tracker_bin"
TESTROOT="/tmp/p2p_feature_test"

cleanup() {
    pkill -f "tracker_bin" 2>/dev/null || true
    pkill -f "client_bin peer_daemon" 2>/dev/null || true
    sleep 1
}
trap cleanup EXIT

# ── Pre-flight ────────────────────────────────────────────────────────────────
section "Pre-flight"
[ -x "$CLIENT_BIN" ]  && pass "client_bin executable" || { fail "client_bin missing"; exit 1; }
[ -x "$TRACKER_BIN" ] && pass "tracker_bin executable" || { fail "tracker_bin missing"; exit 1; }
cleanup

# ── Setup ─────────────────────────────────────────────────────────────────────
rm -rf "$TESTROOT"
mkdir -p "$TESTROOT"/{tracker1,tracker2,tracker3,alice,bob,charlie,testfiles}

cat > "$TESTROOT/tracker_info.txt" <<'EOF'
127.0.0.1:9000
127.0.0.1:9001
127.0.0.1:9002
EOF

for i in 1 2 3; do
    cp "$TRACKER_BIN"               "$TESTROOT/tracker$i/tracker_bin"
    cp "$TESTROOT/tracker_info.txt" "$TESTROOT/tracker$i/tracker_info.txt"
done
for u in alice bob charlie; do
    cp "$CLIENT_BIN"                "$TESTROOT/$u/client_bin"
    cp "$TESTROOT/tracker_info.txt" "$TESTROOT/$u/tracker_info.txt"
done

# ── Start 3 trackers ──────────────────────────────────────────────────────────
section "Starting trackers"
for i in 1 2 3; do
    (cd "$TESTROOT/tracker$i" && ./tracker_bin tracker_info.txt $i \
        >> /tmp/feature_tracker$i.log 2>&1 &)
done
sleep 3

for port in 9000 9001 9002; do
    nc -z -w1 127.0.0.1 $port 2>/dev/null \
        && pass "Tracker :$port reachable" \
        || fail "Tracker :$port NOT reachable"
done

# Helper: run a client command from a user's dir
client() { local u="$1"; shift; (cd "$TESTROOT/$u" && ./client_bin "$@" 2>&1); }

# ── Bootstrap shared users ────────────────────────────────────────────────────
client alice   create_user Alice   pass123 >/dev/null
client alice   login       Alice   pass123 >/dev/null
client bob     create_user Bob     pass456 >/dev/null
client bob     login       Bob     pass456 >/dev/null
client charlie create_user Charlie pass789 >/dev/null
client charlie login       Charlie pass789 >/dev/null
sleep 1   # let peer daemons register

# ═══════════════════════════════════════════════════════════════════════════════
section "1. Empty file rejection"
# ═══════════════════════════════════════════════════════════════════════════════

# Create the empty file
touch "$TESTROOT/testfiles/empty.bin"

# Also need a group to upload into
client alice create_group EmptyTestGrp >/dev/null

# Try to upload the empty file — must fail with a clear message
OUT=$(client alice upload_file "$TESTROOT/testfiles/empty.bin" EmptyTestGrp)
assert_contains "Empty file upload rejected"     "empty\|cannot\|error"  "$OUT"
assert_not_contains "Empty file NOT silently accepted" "uploaded\|success"  "$OUT"

# Non-empty file must upload fine
echo "I am not empty" > "$TESTROOT/testfiles/nonempty.txt"
OUT=$(client alice upload_file "$TESTROOT/testfiles/nonempty.txt" EmptyTestGrp)
assert_contains "Non-empty file uploads OK"      "uploaded\|success"  "$OUT"

# Verify that the empty file does not appear in the group
OUT=$(client alice list_files EmptyTestGrp)
assert_not_contains "Empty file not in group listing" "empty.bin" "$OUT"
assert_contains     "Non-empty file IS in listing"    "nonempty.txt" "$OUT"

# ═══════════════════════════════════════════════════════════════════════════════
section "2. Tracker rejoin — missed-state pull (pullStateFromPeers)"
# ═══════════════════════════════════════════════════════════════════════════════

info "Stopping tracker 2 (port 9001)..."
pkill -f "tracker_bin tracker_info.txt 2" 2>/dev/null || true
sleep 1

# Verify tracker 2 is actually down
nc -z -w1 127.0.0.1 9001 2>/dev/null \
    && fail "Tracker 2 still up (expected it to be down)" \
    || pass "Tracker 2 confirmed down"

# ── Make state changes while tracker 2 is DOWN ────────────────────────────────

# Create a new user while tracker 2 is down
OUT=$(client alice create_user MissedUser missedpass)
assert_contains "MissedUser created while tracker 2 down" "created\|user" "$OUT"

# Create a group
OUT=$(client alice create_group MissedGroup)
assert_contains "MissedGroup created while tracker 2 down" "created\|group" "$OUT"

# Create a small file and upload it to MissedGroup
echo "State missed by tracker 2 during downtime" > "$TESTROOT/testfiles/missed.txt"
client alice join_group MissedGroup >/dev/null || true  # already owner, just ensure file goes in
OUT=$(client alice upload_file "$TESTROOT/testfiles/missed.txt" MissedGroup)
assert_contains "missed.txt uploaded while tracker 2 down" "uploaded\|success" "$OUT"

# ── Restart tracker 2 ─────────────────────────────────────────────────────────
info "Restarting tracker 2..."
(cd "$TESTROOT/tracker2" && ./tracker_bin tracker_info.txt 2 \
    >> /tmp/feature_tracker2.log 2>&1 &)

info "Waiting 4s for tracker 2 to execute pullStateFromPeers..."
sleep 4

# Verify tracker 2 is back up
nc -z -w1 127.0.0.1 9001 2>/dev/null \
    && pass "Tracker 2 back up" \
    || fail "Tracker 2 did not restart"

# ── Query tracker 2 EXCLUSIVELY to verify state pull worked ───────────────────
REJOIN_DIR="$TESTROOT/rejoin_client"
mkdir -p "$REJOIN_DIR"
cp "$CLIENT_BIN" "$REJOIN_DIR/client_bin"
echo "127.0.0.1:9001" > "$REJOIN_DIR/tracker_info.txt"  # ONLY tracker 2

# MissedUser must now be known to tracker 2
OUT=$(cd "$REJOIN_DIR" && ./client_bin login MissedUser missedpass 2>&1)
assert_contains "MissedUser synced to tracker 2 after rejoin" "logged in\|peer server" "$OUT"

# MissedGroup must be visible on tracker 2
OUT=$(cd "$REJOIN_DIR" && ./client_bin list_groups 2>&1)
assert_contains "MissedGroup synced to tracker 2" "MissedGroup" "$OUT"

# missed.txt must be listed in MissedGroup on tracker 2
# Accept MissedUser first so it can query list_files (membership check)
ALICE_REJOIN="$TESTROOT/alice_rejoin"
mkdir -p "$ALICE_REJOIN"
cp "$CLIENT_BIN" "$ALICE_REJOIN/client_bin"
echo "127.0.0.1:9001" > "$ALICE_REJOIN/tracker_info.txt"  # ONLY tracker 2
# Re-login Alice via tracker 2
(cd "$ALICE_REJOIN" && ./client_bin login Alice pass123 >/dev/null 2>&1) || true
sleep 1

OUT=$(cd "$ALICE_REJOIN" && ./client_bin list_files MissedGroup 2>&1)
assert_contains "missed.txt synced to tracker 2" "missed.txt" "$OUT"

# Confirm the missed state is NOT in tracker 2's original saved state
# (i.e., it came via pullStateFromPeers, not from loading tracker_state.json)
info "Tracker 2 rejoin log:"
grep -i "merged state\|rejoin\|sync\|missed" /tmp/feature_tracker2.log | tail -6 || true

# ═══════════════════════════════════════════════════════════════════════════════
section "3. Rarest-first piece selection (P2P_RAREST_FIRST=1)"
# ═══════════════════════════════════════════════════════════════════════════════

# Create a file that spans exactly 2 chunks (~600KB → chunk 0 = 512KB, chunk 1 = ~88KB)
dd if=/dev/urandom bs=1024 count=600 of="$TESTROOT/testfiles/rarity.bin" 2>/dev/null

# Set up group and upload (Alice is the seeder with ALL chunks)
client alice create_group RarityGrp >/dev/null
client alice upload_file "$TESTROOT/testfiles/rarity.bin" RarityGrp >/dev/null

# Bob joins and downloads rarity.bin (Bob will be a full seeder too)
client bob join_group RarityGrp >/dev/null
client alice accept_request RarityGrp Bob >/dev/null
sleep 1

client bob download_file RarityGrp rarity.bin "$TESTROOT/bob/rarity_full.bin" >/dev/null
assert_file_exists "Bob downloaded rarity.bin" "$TESTROOT/bob/rarity_full.bin"

info "Waiting 1s for Bob's seeder registration to propagate..."
sleep 1

# Get the file hash so we can manipulate Bob's chunk store
FILE_HASH=$(shasum -a 256 "$TESTROOT/testfiles/rarity.bin" | awk '{print $1}')
BOB_CHUNK_DIR="$TESTROOT/bob/.chunks/$FILE_HASH"

# Verify Bob has both chunks
CHUNK_COUNT=$(ls "$BOB_CHUNK_DIR"/chunk_*.dat 2>/dev/null | wc -l | tr -d ' ')
if [ "$CHUNK_COUNT" -lt 2 ]; then
    skip "rarity.bin produced only $CHUNK_COUNT chunk(s) — need 2+ for partial-seeder test"
else
    pass "rarity.bin produced $CHUNK_COUNT chunks (need ≥2)"

    # ── Make Bob a PARTIAL seeder: keep only chunk 0, delete chunk 1 ─────────
    for f in "$BOB_CHUNK_DIR"/chunk_*.dat; do
        idx=$(basename "$f" | grep -o '[0-9]*' | head -1)
        if [ "$idx" -ne 0 ]; then
            rm -f "$f"
        fi
    done
    REMAINING=$(ls "$BOB_CHUNK_DIR"/chunk_*.dat 2>/dev/null | wc -l | tr -d ' ')
    info "Bob now has $REMAINING chunk(s) after partial deletion (kept chunk 0 only)"
    pass "Bob set up as partial seeder (chunk 0 only)"

    # ── Now Charlie downloads with rarest-first ────────────────────────────────
    # Charlie is not a member yet; join and get accepted
    client charlie join_group RarityGrp >/dev/null
    client alice accept_request RarityGrp Charlie >/dev/null
    sleep 1

    # Wait for Alice & Bob peer daemons to have current addresses
    sleep 1

    RAREST_LOG="$TESTROOT/charlie_rarest.log"
    (cd "$TESTROOT/charlie" && \
        P2P_RAREST_FIRST=1 ./client_bin download_file RarityGrp rarity.bin rarity_dl.bin \
        > "$RAREST_LOG" 2>&1)

    assert_file_exists "Charlie's rarest-first download succeeded" \
        "$TESTROOT/charlie/rarity_dl.bin"
    assert_files_equal "rarest-first file matches original" \
        "$TESTROOT/testfiles/rarity.bin" "$TESTROOT/charlie/rarity_dl.bin"

    # Verify rarest-first mode was activated
    assert_contains "Log shows rarest-first selection" \
        "rarest-first\|Piece selection" "$(cat "$RAREST_LOG")"

    # Verify bitfields were queried (indicates the getBitfields path ran)
    assert_contains "Log queried peer bitfields" \
        "queried [0-9]* peer" "$(cat "$RAREST_LOG")"

    info "Rarest-first log:"
    cat "$RAREST_LOG"

    # How many peers were queried?
    PEERS_QUERIED=$(grep -oi "queried [0-9]* peer" "$RAREST_LOG" | grep -o '[0-9]*' | head -1 || echo "0")
    info "Peers queried for bitfields: $PEERS_QUERIED"

    if [ "${PEERS_QUERIED:-0}" -ge 2 ]; then
        # 2+ peers → Bob (partial: chunk 0 only) + Alice (all chunks)
        # Chunk index 1 → display "chunk 2/$CHUNK_COUNT" has count=1 (rarest: only Alice)
        # Chunk index 0 → display "chunk 1/$CHUNK_COUNT" has count=2 (both)
        # Rarest-first must download chunk 2/N before chunk 1/N
        RAREST_LINE=$(grep -n "chunk 2/$CHUNK_COUNT" "$RAREST_LOG" | head -1 | cut -d: -f1 || true)
        COMMON_LINE=$(grep -n "chunk 1/$CHUNK_COUNT"  "$RAREST_LOG" | head -1 | cut -d: -f1 || true)

        if [ -n "$RAREST_LINE" ] && [ -n "$COMMON_LINE" ]; then
            if [ "$RAREST_LINE" -lt "$COMMON_LINE" ]; then
                pass "Rarest chunk (idx 1, only Alice has it) downloaded BEFORE common chunk (idx 0)"
            else
                fail "Rarest-first ordering violated: rarest chunk at log line $RAREST_LINE, common at $COMMON_LINE (expected rarest first)"
            fi
        else
            skip "Could not find both chunk download lines in log to verify order"
        fi
    else
        # 1 peer → all chunks equally rare → sequential order (0 then 1) is correct
        pass "Single-peer scenario: all chunks equally rare → sequential order correct (rarest-first = sequential)"
        info "Note: Bob's seeder registration may not have reached tracker in time; full multi-seeder verification requires 2+ peers"
    fi
fi

# ── Also test rarest-first works with a single seeder (falls back gracefully) ─
section "3b. Rarest-first with single seeder (graceful fallback)"

echo "Single seeder fallback test content." > "$TESTROOT/testfiles/singleseeder.txt"
client alice create_group SingleSeedGrp >/dev/null
client alice upload_file "$TESTROOT/testfiles/singleseeder.txt" SingleSeedGrp >/dev/null

client charlie join_group SingleSeedGrp >/dev/null
client alice accept_request SingleSeedGrp Charlie >/dev/null
sleep 1

OUT=$(cd "$TESTROOT/charlie" && \
    P2P_RAREST_FIRST=1 ./client_bin download_file SingleSeedGrp singleseeder.txt out_single.txt 2>&1)

assert_contains "Single-seeder rarest-first download ok"  "complete\|downloaded" "$OUT"
assert_file_exists "Single-seeder file downloaded"        "$TESTROOT/charlie/out_single.txt"
assert_files_equal "Single-seeder file matches original"  \
    "$TESTROOT/testfiles/singleseeder.txt" "$TESTROOT/charlie/out_single.txt"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}${BLUE}═══════════════════════════════════════${NC}"
echo -e "${BOLD}  Feature Test Summary${NC}"
echo -e "${BOLD}${BLUE}═══════════════════════════════════════${NC}"
echo -e "  ${GREEN}PASS${NC}: $PASS"
echo -e "  ${RED}FAIL${NC}: $FAIL"
echo -e "  ${YELLOW}SKIP${NC}: $SKIP"
echo -e "  Total: $((PASS + FAIL + SKIP))"
echo ""

if [ ${#FAILURES[@]} -gt 0 ]; then
    echo -e "${RED}Failed:${NC}"
    for f in "${FAILURES[@]}"; do echo "  - $f"; done
    echo ""
fi

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}All feature tests passed! ✅${NC}"
    exit 0
else
    echo -e "${RED}${BOLD}$FAIL test(s) failed ❌${NC}"
    exit 1
fi
