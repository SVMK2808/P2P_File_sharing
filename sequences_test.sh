#!/usr/bin/env bash
# sequences_test.sh — exhaustive event-sequence tests for the P2P codebase.
#
# Covers 18 named sequences:
#   SEQ-1   Single-user full lifecycle
#   SEQ-2   Two-user join/accept/download/seeder-handoff
#   SEQ-3   Permission enforcement (non-member, pending, non-owner)
#   SEQ-4   Duplicate-operation errors (user/group/file)
#   SEQ-5   Owner edge-cases (owner cannot leave, member can)
#   SEQ-6   Rejoin after leave
#   SEQ-7   Last-seeder removal makes file unavailable
#   SEQ-8   Tracker persistence across restart
#   SEQ-9   Resume download (partial → restart)
#   SEQ-10  Multi-tracker failover
#   SEQ-11  Rarest-first vs sequential ordering
#   SEQ-12  Wrong-password and unknown commands
#   SEQ-13  Empty-file upload rejection
#   SEQ-14  Multi-file group — independent file operations
#   SEQ-15  show_downloads reflects local chunk state
#   SEQ-16  Status command reflects login state
#   SEQ-17  list_groups reflects created groups
#   SEQ-18  DHT state sync — late-joining tracker learns state
#
# Tracker startup API: ./tracker_bin <config_file> <line_number>
#
# Usage:  bash sequences_test.sh

set -uo pipefail

# ─── Paths ────────────────────────────────────────────────────────────────────
REPO="$(cd "$(dirname "$0")" && pwd)"
CLIENT="$REPO/client_bin"
TRACKER="$REPO/tracker_bin"
TMPDIR_BASE="$REPO/.seq_test_tmp"

# ─── Counters ─────────────────────────────────────────────────────────────────
PASS=0; FAIL=0; SKIP=0
FAILED_TESTS=()

# ─── Colours ──────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# ─── Tracker bookkeeping ──────────────────────────────────────────────────────
# TRACKER_IDX never resets — each section gets a unique directory so old and new
# tracker processes never share the same tracker_state.json.
TRACKER_IDX=0
declare -a TRACKER_PIDS=()     # only the CURRENT section's live PIDs

BASE_PORT=19100   # well away from the hardcoded 9000-9002 in full_test.sh

# ─── Core helpers ─────────────────────────────────────────────────────────────
ok()      { PASS=$((PASS+1)); echo -e "  ${GREEN}✓${NC} $1"; }
fail_t()  { FAIL=$((FAIL+1)); FAILED_TESTS+=("$1"); echo -e "  ${RED}✗${NC} $1: expected '$2' / got '$(echo "${3:-}" | head -1)'"; }
info()    { echo -e "  ${CYAN}»${NC} $1"; }
section() { echo; echo -e "${BOLD}${CYAN}═══ $1 ═══${NC}"; }

# Run a client command from a specific working directory (never exits non-zero).
run() {
    local dir=$1; shift
    (cd "$dir" && "$CLIENT" "$@" 2>&1) || true
}

# assert_ok  <name> <expected-ERE-regex> <actual>
assert_ok() {
    if echo "${3:-}" | grep -qiE "${2}"; then ok "$1"; else fail_t "$1" "$2" "${3:-}"; fi
}

# assert_fail  <name> <bad-regex> <actual>   — passes when regex does NOT match
assert_fail() {
    if echo "${3:-}" | grep -qiE "${2}"; then
        fail_t "$1  (should NOT match '$2')" "NO $2" "${3:-}"
    else
        ok "$1"
    fi
}

# ─── Tracker lifecycle ────────────────────────────────────────────────────────
# IMPORTANT: We use   (cd dir && exec CMD) &   so that $! is the PID of CMD
# itself (not a wrapper bash subshell).  This means kill $! terminates the
# actual tracker process instead of leaving it as an orphan.

# start_tracker PORT
#   Starts a standalone, single-tracker instance on $PORT.
start_tracker() {
    local port=$1
    TRACKER_IDX=$((TRACKER_IDX+1))
    local tdir="$TMPDIR_BASE/tracker_$TRACKER_IDX"
    mkdir -p "$tdir"
    echo "127.0.0.1:$port" > "$tdir/tracker_info.txt"

    # exec replaces the bash subshell → $! = tracker_bin PID
    (cd "$tdir" && exec "$TRACKER" tracker_info.txt 1 > tracker.log 2>&1) &
    TRACKER_PIDS+=("$!")
    sleep 1.0   # wait for bind + ready
}

# start_tracker_cluster PORT1 PORT2 ...
#   Starts N trackers that each know about one another.
start_tracker_cluster() {
    local ports=("$@")
    TRACKER_IDX=$((TRACKER_IDX+1))
    local base_idx=$TRACKER_IDX
    local cfg="$TMPDIR_BASE/cluster_cfg_$base_idx.txt"
    for p in "${ports[@]}"; do echo "127.0.0.1:$p" >> "$cfg"; done

    local line=1
    for p in "${ports[@]}"; do
        local tdir="$TMPDIR_BASE/tracker_${TRACKER_IDX}"
        mkdir -p "$tdir"
        # Use absolute path  for cfg since we cd into tdir
        (cd "$tdir" && exec "$TRACKER" "$cfg" "$line" > tracker.log 2>&1) &
        TRACKER_PIDS+=("$!")
        TRACKER_IDX=$((TRACKER_IDX+1))
        line=$((line+1))
    done
    sleep 1.2   # wait for initial gossip exchange
}

# kill_tracker IDX — kill a tracker by 0-based index into the CURRENT section's
#                    TRACKER_PIDS array.
kill_tracker() {
    local pid="${TRACKER_PIDS[$1]:-}"
    [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
    TRACKER_PIDS[$1]=""
}

# stop_section_trackers — kill every tracker started in the CURRENT section and
#                         reset the per-section array.
stop_section_trackers() {
    for pid in "${TRACKER_PIDS[@]}"; do
        [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
    done
    TRACKER_PIDS=()
    # Also kill any peer daemons spawned during this section to avoid buildup
    pkill -f "client_bin peer_daemon" 2>/dev/null || true
    sleep 0.4
}

# make_client_dir LABEL PORT [PORT ...]
#   Creates a fresh working directory pre-seeded with tracker_info.txt.
make_client_dir() {
    local name=$1; shift
    local dir="$TMPDIR_BASE/client_$name"
    mkdir -p "$dir"
    : > "$dir/tracker_info.txt"
    for p in "$@"; do echo "127.0.0.1:$p" >> "$dir/tracker_info.txt"; done
    echo "$dir"
}

# ─── Login helpers ────────────────────────────────────────────────────────────
# login_user DIR USER PASS — logs in and asserts success; exits script on failure
login_user() {
    local dir=$1 user=$2 pass=$3
    local out
    out=$(run "$dir" login "$user" "$pass")
    if echo "$out" | grep -qiE "ok|logged in"; then
        return 0
    else
        echo -e "  ${RED}FATAL${NC}: login '$user' failed: $out"
        stop_section_trackers
        exit 1
    fi
}

# ─── Test-data helpers ────────────────────────────────────────────────────────
make_file()  { dd if=/dev/urandom bs=1024 count="${2:-8}" 2>/dev/null | base64 > "$1"; }
checksum()   { shasum -a 256 "$1" | awk '{print $1}'; }

# ─── Global cleanup ───────────────────────────────────────────────────────────
cleanup() {
    for pid in "${TRACKER_PIDS[@]}"; do
        [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
    done
    pkill -f "client_bin peer_daemon" 2>/dev/null || true
    sleep 0.2
    [[ -d "$TMPDIR_BASE" ]] && rm -rf "$TMPDIR_BASE"
}
trap cleanup EXIT INT TERM

# ════════════════════════════════════════════════════════════════════════════════
rm -rf "$TMPDIR_BASE"; mkdir -p "$TMPDIR_BASE"
echo
echo -e "${BOLD}P2P Sequences Test Suite${NC}"
echo -e "Repo: $REPO"
echo

section "Pre-flight"
[[ -x "$CLIENT"  ]] && ok "client_bin present"  || { echo "FATAL: $CLIENT missing"; exit 1; }
[[ -x "$TRACKER" ]] && ok "tracker_bin present" || { echo "FATAL: $TRACKER missing"; exit 1; }

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-1: Single-user full lifecycle"
# create_user → login → create_group → upload → list_files → download
# → stop_sharing → logout → re-login → list_files still works
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A1=$(make_client_dir "seq1_alice" $PORT)

assert_ok "1.1 create_user alice1" "user created" \
    "$(run "$A1" create_user alice1 pass1)"

assert_ok "1.2 login alice1" "ok" \
    "$(run "$A1" login alice1 pass1)"
sleep 0.6

assert_ok "1.3 create_group grp1" "created" \
    "$(run "$A1" create_group grp1)"

echo "Hello P2P world" > "$A1/hello.txt"
assert_ok "1.4 upload hello.txt" "uploaded" \
    "$(run "$A1" upload_file hello.txt grp1)"

assert_ok "1.5 list_files shows hello.txt" "hello" \
    "$(run "$A1" list_files grp1)"

assert_ok "1.6 download hello.txt" "complete" \
    "$(run "$A1" download_file grp1 hello.txt hello_dl.txt)"

assert_ok "1.7 stop_sharing hello.txt" "stop|removed|sharing" \
    "$(run "$A1" stop_sharing grp1 hello.txt)"

assert_ok "1.8 logout" "logged out" \
    "$(run "$A1" logout)"

assert_ok "1.9 re-login succeeds" "ok" \
    "$(run "$A1" login alice1 pass1)"
sleep 0.4

assert_ok "1.10 list_files after re-login (group persists)" \
    "hello|no files|ok" \
    "$(run "$A1" list_files grp1)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-2: Two-user join → accept → download → seeder handoff"
# Alice creates group and uploads; Bob joins and is accepted; Bob downloads
# (auto-registers as seeder); Alice stops sharing; Charlie (late joiner)
# downloads successfully from Bob.
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A2=$(make_client_dir "seq2_alice"   $PORT)
B2=$(make_client_dir "seq2_bob"     $PORT)
C2=$(make_client_dir "seq2_charlie" $PORT)

run "$A2" create_user alice2   pass2 > /dev/null
run "$B2" create_user bob2     pass2 > /dev/null
run "$C2" create_user charlie2 pass2 > /dev/null
login_user "$A2" alice2   pass2; sleep 0.6
login_user "$B2" bob2     pass2; sleep 0.6
login_user "$C2" charlie2 pass2; sleep 0.6

run "$A2" create_group grp2 > /dev/null

SRCFILE2="$A2/data2.txt"
make_file "$SRCFILE2" 12
run "$A2" upload_file data2.txt grp2 > /dev/null

assert_ok "2.1 Bob requests join" "request|ok" \
    "$(run "$B2" join_group grp2)"

assert_ok "2.2 Alice lists pending – sees bob2" "bob2" \
    "$(run "$A2" list_requests grp2)"

assert_ok "2.3 Alice accepts Bob" "accepted" \
    "$(run "$A2" accept_request grp2 bob2)"

assert_ok "2.4 Bob downloads data2.txt" "complete" \
    "$(run "$B2" download_file grp2 data2.txt data2_dl.txt)"
sleep 0.5   # let add_seeder propagate

assert_ok "2.5 Alice stops sharing" "stop|removed" \
    "$(run "$A2" stop_sharing grp2 data2.txt)"

run "$C2" join_group grp2 > /dev/null
run "$A2" accept_request grp2 charlie2 > /dev/null

assert_ok "2.6 Charlie downloads from Bob (seeder handoff)" "complete" \
    "$(run "$C2" download_file grp2 data2.txt data2_charlie.txt)"

SRC2=$(checksum "$SRCFILE2")
assert_ok "2.7 Bob's copy matches original" "x" \
    "$([ "$SRC2" = "$(checksum "$B2/data2_dl.txt")" ] && echo x || echo mismatch)"
assert_ok "2.8 Charlie's copy matches original" "x" \
    "$([ "$SRC2" = "$(checksum "$C2/data2_charlie.txt")" ] && echo x || echo mismatch)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-3: Permission enforcement"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A3=$(make_client_dir "seq3_alice"   $PORT)
B3=$(make_client_dir "seq3_bob"     $PORT)
C3=$(make_client_dir "seq3_charlie" $PORT)

run "$A3" create_user alice3   pass3 > /dev/null
run "$B3" create_user bob3     pass3 > /dev/null
run "$C3" create_user charlie3 pass3 > /dev/null
login_user "$A3" alice3   pass3; sleep 0.6
login_user "$B3" bob3     pass3; sleep 0.6
login_user "$C3" charlie3 pass3; sleep 0.5

run "$A3" create_group grp3 > /dev/null
echo "secret" > "$A3/secret.txt"
run "$A3" upload_file secret.txt grp3 > /dev/null

assert_ok "3.1 non-member blocked from list_files" "error|not a member" \
    "$(run "$B3" list_files grp3)"

echo "intruder" > "$B3/intruder.txt"
assert_ok "3.2 non-member blocked from upload_file" "error|not a member" \
    "$(run "$B3" upload_file intruder.txt grp3)"

run "$B3" join_group grp3 > /dev/null   # pending — not yet accepted

assert_ok "3.3 pending user still cannot list_files" "error|not a member" \
    "$(run "$B3" list_files grp3)"

assert_ok "3.4 non-member download attempt fails" \
    "error|not a member|no peer|failed" \
    "$(run "$B3" download_file grp3 secret.txt steal.txt)"

assert_ok "3.5 non-owner cannot list_requests" "error|not owner" \
    "$(run "$B3" list_requests grp3)"

assert_ok "3.6 third party cannot accept_request" "error|not owner" \
    "$(run "$C3" accept_request grp3 bob3)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-4: Duplicate-operation errors"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A4=$(make_client_dir "seq4_alice" $PORT)

run "$A4" create_user alice4 pass4 > /dev/null
assert_ok "4.1 duplicate create_user fails" "error|user exists" \
    "$(run "$A4" create_user alice4 pass4)"

login_user "$A4" alice4 pass4; sleep 0.4
run "$A4" create_group grp4 > /dev/null
assert_ok "4.2 duplicate create_group fails" "error|group exists" \
    "$(run "$A4" create_group grp4)"

echo "original" > "$A4/dup.txt"
run "$A4" upload_file dup.txt grp4 > /dev/null
assert_ok "4.3 duplicate upload_file fails" "error|already exists" \
    "$(run "$A4" upload_file dup.txt grp4)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-5: Owner edge cases"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A5=$(make_client_dir "seq5_alice" $PORT)
B5=$(make_client_dir "seq5_bob"   $PORT)

run "$A5" create_user alice5 pass5 > /dev/null
run "$B5" create_user bob5   pass5 > /dev/null
login_user "$A5" alice5 pass5; sleep 0.5
login_user "$B5" bob5   pass5; sleep 0.4

run "$A5" create_group grp5 > /dev/null

assert_ok "5.1 owner cannot leave own group" "error|owner cannot" \
    "$(run "$A5" leave_group grp5)"

run "$B5" join_group grp5 > /dev/null
run "$A5" accept_request grp5 bob5 > /dev/null

assert_ok "5.2 member (Bob) can leave group" "left|ok" \
    "$(run "$B5" leave_group grp5)"

assert_ok "5.3 ex-member cannot list_files" "error|not a member" \
    "$(run "$B5" list_files grp5)"

assert_ok "5.4 leaving a second time is an error" "error|not a member" \
    "$(run "$B5" leave_group grp5)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-6: Rejoin after leave"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A6=$(make_client_dir "seq6_alice" $PORT)
B6=$(make_client_dir "seq6_bob"   $PORT)

run "$A6" create_user alice6 pass6 > /dev/null
run "$B6" create_user bob6   pass6 > /dev/null
login_user "$A6" alice6 pass6; sleep 0.5
login_user "$B6" bob6   pass6; sleep 0.4

run "$A6" create_group grp6 > /dev/null
echo "shared" > "$A6/shared.txt"
run "$A6" upload_file shared.txt grp6 > /dev/null
run "$B6" join_group grp6 > /dev/null
run "$A6" accept_request grp6 bob6 > /dev/null

assert_ok "6.1 Bob can list_files as member" "shared" \
    "$(run "$B6" list_files grp6)"

run "$B6" leave_group grp6 > /dev/null

assert_ok "6.2 Bob denied after leave" "error|not a member" \
    "$(run "$B6" list_files grp6)"

run "$B6" join_group grp6 > /dev/null
run "$A6" accept_request grp6 bob6 > /dev/null

assert_ok "6.3 Bob can list again after rejoin + accept" "shared" \
    "$(run "$B6" list_files grp6)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-7: Last-seeder removal makes file unavailable"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A7=$(make_client_dir "seq7_alice"   $PORT)
B7=$(make_client_dir "seq7_bob"     $PORT)
C7=$(make_client_dir "seq7_charlie" $PORT)

run "$A7" create_user alice7   pass7 > /dev/null
run "$B7" create_user bob7     pass7 > /dev/null
run "$C7" create_user charlie7 pass7 > /dev/null
login_user "$A7" alice7   pass7; sleep 0.5
login_user "$B7" bob7     pass7; sleep 0.5
login_user "$C7" charlie7 pass7; sleep 0.5

run "$A7" create_group grp7 > /dev/null
make_file "$A7/ephemeral.dat" 6
run "$A7" upload_file ephemeral.dat grp7 > /dev/null

run "$B7" join_group grp7 > /dev/null; run "$A7" accept_request grp7 bob7     > /dev/null
run "$C7" join_group grp7 > /dev/null; run "$A7" accept_request grp7 charlie7 > /dev/null

run "$B7" download_file grp7 ephemeral.dat eph_b.dat > /dev/null; sleep 0.5

assert_ok "7.1 Alice stops sharing" "stop|removed" \
    "$(run "$A7" stop_sharing grp7 ephemeral.dat)"

assert_ok "7.2 Bob (last seeder) stops sharing — file deleted" \
    "stop|removed|no owners" \
    "$(run "$B7" stop_sharing grp7 ephemeral.dat)"

assert_ok "7.3 Charlie's download fails — no peers / file gone" \
    "error|no peer|not found|failed" \
    "$(run "$C7" download_file grp7 ephemeral.dat eph_c.dat)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-8: Tracker persistence across restart"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
TDIR8="${TMPDIR_BASE}/tracker_${TRACKER_IDX}"   # dir of the most-recently started tracker
A8=$(make_client_dir "seq8_alice" $PORT)

run "$A8" create_user alice8 pass8 > /dev/null
login_user "$A8" alice8 pass8; sleep 0.5
run "$A8" create_group grp8 > /dev/null
echo "persisted content" > "$A8/persist.txt"
run "$A8" upload_file persist.txt grp8 > /dev/null
sleep 0.4  # let SaveState async commit to disk

info "Killing tracker (simulating crash)..."
kill_tracker 0; sleep 0.6  # wait for graceful shutdown + SaveState

info "Restarting tracker from same dir (picks up tracker_state.json)..."
(cd "$TDIR8" && exec "$TRACKER" tracker_info.txt 1 > tracker2.log 2>&1) &
TRACKER_PIDS+=("$!")
sleep 1.0  # let tracker load and become ready

assert_ok "8.1 list_files after restart sees persist.txt" "persist" \
    "$(run "$A8" list_files grp8)"

assert_ok "8.2 login after restart succeeds (user persisted)" "ok" \
    "$(run "$A8" login alice8 pass8)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-9: Resume download (partial → restart)"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A9=$(make_client_dir "seq9_alice" $PORT)
B9=$(make_client_dir "seq9_bob"   $PORT)

run "$A9" create_user alice9 pass9 > /dev/null
run "$B9" create_user bob9   pass9 > /dev/null
login_user "$A9" alice9 pass9; sleep 0.5
login_user "$B9" bob9   pass9; sleep 0.4

run "$A9" create_group grp9 > /dev/null
make_file "$A9/big.dat" 1100   # > 1 MiB → multiple 512 KiB chunks
run "$A9" upload_file big.dat grp9 > /dev/null
run "$B9" join_group grp9 > /dev/null
run "$A9" accept_request grp9 bob9 > /dev/null

assert_ok "9.1 First full download completes" "complete" \
    "$(run "$B9" download_file grp9 big.dat big_dl.dat)"
sleep 0.4   # let add_seeder propagate

# Remove Bob from the seeder list so the resume download only uses Alice.
# This prevents round-robin from picking Bob (who has only chunk_0 on disk).
run "$B9" stop_sharing grp9 big.dat > /dev/null

FILE_HASH9=$(ls "$B9/.chunks/" 2>/dev/null | head -1)
CHUNK_DIR9="$B9/.chunks/$FILE_HASH9"

if [[ -n "$FILE_HASH9" && -d "$CHUNK_DIR9" ]]; then
    # Simulate partial download by deleting all chunks except chunk_0
    for f in "$CHUNK_DIR9"/chunk_*.dat; do
        [[ "$f" == *"/chunk_0.dat" ]] || rm -f "$f"
    done
    rm -f "$B9/big_dl.dat"
    # Re-register as seeder so tracker still has the file entry with Alice seeding
    # (stop_sharing removed Bob; Alice is still set as owner from upload)

    RESUME9=$(run "$B9" download_file grp9 big.dat big_dl_resume.dat)
    assert_ok "9.2 Resume download completes"      "complete" "$RESUME9"
    assert_ok "9.3 Resume output mentions skipped" "skip"     "$RESUME9"

    SRC9=$(checksum "$A9/big.dat")
    DL9F=$(checksum "$B9/big_dl_resume.dat")
    assert_ok "9.4 Resumed file matches original" "x" \
        "$([ "$SRC9" = "$DL9F" ] && echo x || echo mismatch)"
else
    SKIP=$((SKIP+3)); info "9.2-9.4 skipped: chunk dir not found"
fi

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-10: Multi-tracker failover"
# ════════════════════════════════════════════════════════════════════════════════
PORT1=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
PORT2=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker_cluster $PORT1 $PORT2

A10=$(make_client_dir "seq10_alice" $PORT1 $PORT2)
run "$A10" create_user alice10 pass10 > /dev/null
login_user "$A10" alice10 pass10; sleep 0.5
run "$A10" create_group grp10 > /dev/null
echo "failover test" > "$A10/failover.txt"
run "$A10" upload_file failover.txt grp10 > /dev/null
sleep 0.6   # let sync propagate to T2

assert_ok "10.1 list_files with both trackers up" "failover" \
    "$(run "$A10" list_files grp10)"

info "Killing tracker 1 (index 0)..."
kill_tracker 0; sleep 0.6

assert_ok "10.2 list_files after T1 killed (failover to T2)" "failover" \
    "$(run "$A10" list_files grp10)"

B10=$(make_client_dir "seq10_bob" $PORT1 $PORT2)
assert_ok "10.3 create_user via T2 only" "user created" \
    "$(run "$B10" create_user bob10b pass10)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-11: Rarest-first vs sequential ordering"
# Alice seeds full file (2 chunks).  Bob downloads full, then chuck_1 stripped
# so Bob only seeds chunk_0.  Dave downloads with P2P_RAREST_FIRST=1.
# Eve downloads sequentially as control.
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A11=$(make_client_dir "seq11_alice" $PORT)
B11=$(make_client_dir "seq11_bob"   $PORT)
D11=$(make_client_dir "seq11_dave"  $PORT)
E11=$(make_client_dir "seq11_eve"   $PORT)

run "$A11" create_user alice11 pass11 > /dev/null
run "$B11" create_user bob11   pass11 > /dev/null
run "$D11" create_user dave11  pass11 > /dev/null
run "$E11" create_user eve11   pass11 > /dev/null
login_user "$A11" alice11 pass11; sleep 0.5
login_user "$B11" bob11   pass11; sleep 0.5
login_user "$D11" dave11  pass11; sleep 0.5
login_user "$E11" eve11   pass11; sleep 0.5

run "$A11" create_group grp11 > /dev/null
make_file "$A11/rarest.dat" 720   # ~720 KiB → 2 × 512 KiB chunks
run "$A11" upload_file rarest.dat grp11 > /dev/null

run "$B11" join_group grp11 > /dev/null
run "$D11" join_group grp11 > /dev/null
run "$E11" join_group grp11 > /dev/null
run "$A11" accept_request grp11 bob11  > /dev/null
run "$A11" accept_request grp11 dave11 > /dev/null
run "$A11" accept_request grp11 eve11  > /dev/null

# Bob becomes a full seeder via download, then we strip chunk_1 to make him partial.
# Eve downloads BEFORE the strip so she gets a clean round-robin (all seeders full).
run "$B11" download_file grp11 rarest.dat rarest_b.dat > /dev/null; sleep 0.5

# Eve downloads sequentially while all seeders are intact (Alice + Bob both full)
SEQ_OUT=$(run "$E11" download_file grp11 rarest.dat rarest_e.dat)
assert_ok "11.5 Sequential mode download completes"          "complete" "$SEQ_OUT"
assert_fail "11.6 Sequential mode does NOT log rarest-first" "rarest-first" "$SEQ_OUT"

# NOW strip Bob's chunk_1 so Bob becomes a partial seeder for the rarest-first test
BOB_HASH11=$(ls "$B11/.chunks/" 2>/dev/null | head -1)
[[ -n "$BOB_HASH11" ]] && rm -f "$B11/.chunks/$BOB_HASH11/chunk_1.dat"

# Dave downloads with rarest-first — chunk_1 (held by fewer peers) should come first
RF_OUT=$(P2P_RAREST_FIRST=1 run "$D11" download_file grp11 rarest.dat rarest_d.dat)
assert_ok "11.1 Dave (rarest-first) download completes"  "complete"            "$RF_OUT"
assert_ok "11.2 Rarest-first mode reported in output"    "rarest"              "$RF_OUT"
assert_ok "11.3 Bitfield query count reported"           "queried|peers|piece" "$RF_OUT"

SRC11=$(checksum "$A11/rarest.dat")
assert_ok "11.4 Dave's file matches original" "x" \
    "$([ "$SRC11" = "$(checksum "$D11/rarest_d.dat")" ] && echo x || echo mismatch)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-12: Wrong password, non-existent user, unknown command"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A12=$(make_client_dir "seq12_alice" $PORT)
run "$A12" create_user alice12 pass12 > /dev/null

assert_ok "12.1 wrong password fails login" "error|invalid" \
    "$(run "$A12" login alice12 WRONG)"

assert_ok "12.2 non-existent user fails login" "error|invalid" \
    "$(run "$A12" login nobody pass12)"

assert_ok "12.3 unknown command returns error" "error|unknown" \
    "$(run "$A12" fly_to_moon)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-13: Empty-file upload rejection"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A13=$(make_client_dir "seq13_alice" $PORT)

run "$A13" create_user alice13 pass13 > /dev/null
login_user "$A13" alice13 pass13; sleep 0.4
run "$A13" create_group grp13 > /dev/null

touch "$A13/empty.txt"
assert_ok "13.1 empty file upload rejected" \
    "error|empty|zero|invalid|0 bytes" \
    "$(run "$A13" upload_file empty.txt grp13)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-14: Multi-file group — independent file operations"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A14=$(make_client_dir "seq14_alice" $PORT)
B14=$(make_client_dir "seq14_bob"   $PORT)

run "$A14" create_user alice14 pass14 > /dev/null
run "$B14" create_user bob14   pass14 > /dev/null
login_user "$A14" alice14 pass14; sleep 0.5
login_user "$B14" bob14   pass14; sleep 0.4

run "$A14" create_group grp14 > /dev/null
echo "content of A" > "$A14/fileA.txt"
echo "content of B" > "$A14/fileB.txt"
run "$A14" upload_file fileA.txt grp14 > /dev/null
run "$A14" upload_file fileB.txt grp14 > /dev/null

run "$B14" join_group grp14 > /dev/null
run "$A14" accept_request grp14 bob14 > /dev/null

LIST14=$(run "$B14" list_files grp14)
assert_ok "14.1 list_files shows fileA" "fileA" "$LIST14"
assert_ok "14.2 list_files shows fileB" "fileB" "$LIST14"

run "$B14" download_file grp14 fileB.txt fileB_dl.txt > /dev/null; sleep 0.4

assert_ok "14.3 Bob stops sharing fileB" "stop|removed" \
    "$(run "$B14" stop_sharing grp14 fileB.txt)"

assert_ok "14.4 fileA still listed after fileB stop" "fileA" \
    "$(run "$B14" list_files grp14)"

assert_ok "14.5 fileB still downloadable from Alice" "complete" \
    "$(run "$B14" download_file grp14 fileB.txt fileB_dl2.txt)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-15: show_downloads reflects local chunk state"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A15=$(make_client_dir "seq15_alice" $PORT)
B15=$(make_client_dir "seq15_bob"   $PORT)

run "$A15" create_user alice15 pass15 > /dev/null
run "$B15" create_user bob15   pass15 > /dev/null
login_user "$A15" alice15 pass15; sleep 0.5
login_user "$B15" bob15   pass15; sleep 0.4

run "$A15" create_group grp15 > /dev/null
echo "chunk test" > "$A15/local.txt"
run "$A15" upload_file local.txt grp15 > /dev/null
run "$B15" join_group grp15 > /dev/null
run "$A15" accept_request grp15 bob15 > /dev/null
run "$B15" download_file grp15 local.txt local_dl.txt > /dev/null; sleep 0.3

assert_ok "15.1 show_downloads lists local.txt for Bob" "local" \
    "$(run "$B15" show_downloads)"

# Both uploader and downloader have local .chunks/ entries, so show_downloads
# also shows Alice's uploaded file chunks.
assert_ok "15.2 show_downloads also appears for uploader (Alice has .chunks)" \
    "local|downloaded" \
    "$(run "$A15" show_downloads)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-16: Status command reflects login state"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A16=$(make_client_dir "seq16_alice" $PORT)
run "$A16" create_user alice16 pass16 > /dev/null

assert_ok "16.1 status before login → not logged in" \
    "not logged|no session|status: not" \
    "$(run "$A16" status)"

login_user "$A16" alice16 pass16; sleep 0.3

assert_ok "16.2 status after login → shows user" "logged in|alice16" \
    "$(run "$A16" status)"

run "$A16" logout > /dev/null

assert_ok "16.3 status after logout → not logged in" \
    "not logged|no session|status: not" \
    "$(run "$A16" status)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-17: list_groups reflects created groups"
# ════════════════════════════════════════════════════════════════════════════════
PORT=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
start_tracker $PORT
A17=$(make_client_dir "seq17_alice" $PORT)
run "$A17" create_user alice17 pass17 > /dev/null
login_user "$A17" alice17 pass17; sleep 0.4

assert_ok "17.1 list_groups on empty tracker" "no groups|no group" \
    "$(run "$A17" list_groups)"

run "$A17" create_group grpAlpha > /dev/null
run "$A17" create_group grpBeta  > /dev/null

LIST17=$(run "$A17" list_groups)
assert_ok "17.2 list_groups shows grpAlpha" "grpAlpha" "$LIST17"
assert_ok "17.3 list_groups shows grpBeta"  "grpBeta"  "$LIST17"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
section "SEQ-18: DHT state sync — late-joining tracker learns existing state"
# Tracker 1 has groups/files.  Tracker 2 starts late and calls
# pullStateFromPeers() to catch up.  A client pointing ONLY at T2 should see T1's state.
# ════════════════════════════════════════════════════════════════════════════════
PORT1=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))
PORT2=$BASE_PORT; BASE_PORT=$((BASE_PORT+1))

# Shared config file listing both trackers.
TRACKER_IDX=$((TRACKER_IDX+1))
CFG18="$TMPDIR_BASE/cluster18_cfg.txt"
echo "127.0.0.1:$PORT1" > "$CFG18"
echo "127.0.0.1:$PORT2" >> "$CFG18"

# Start tracker 1 alone first; build state.
TDIR18_1="$TMPDIR_BASE/tracker_${TRACKER_IDX}"; mkdir -p "$TDIR18_1"
(cd "$TDIR18_1" && exec "$TRACKER" "$CFG18" 1 > tracker.log 2>&1) &
TRACKER_PIDS+=("$!")
sleep 1.0

A18=$(make_client_dir "seq18_alice" $PORT1)
run "$A18" create_user alice18 pass18 > /dev/null
login_user "$A18" alice18 pass18; sleep 0.5
run "$A18" create_group grp18 > /dev/null
echo "sync content" > "$A18/sync.txt"
run "$A18" upload_file sync.txt grp18 > /dev/null
sleep 0.5

info "Starting tracker 2 (late join — should pull state from T1)..."
TRACKER_IDX=$((TRACKER_IDX+1))
TDIR18_2="$TMPDIR_BASE/tracker_${TRACKER_IDX}"; mkdir -p "$TDIR18_2"
(cd "$TDIR18_2" && exec "$TRACKER" "$CFG18" 2 > tracker.log 2>&1) &
TRACKER_PIDS+=("$!")
sleep 1.5   # allow pullStateFromPeers to complete

B18=$(make_client_dir "seq18_bob" $PORT2)   # ONLY tracker 2
run "$B18" create_user bob18 pass18 > /dev/null
login_user "$B18" bob18 pass18; sleep 0.4

assert_ok "18.1 T2 knows grp18 (state pulled from T1 on startup)" "grp18" \
    "$(run "$B18" list_groups)"

stop_section_trackers

# ════════════════════════════════════════════════════════════════════════════════
# SUMMARY
# ════════════════════════════════════════════════════════════════════════════════
TOTAL=$((PASS+FAIL))
echo
echo -e "${BOLD}════════════════════════════════════════${NC}"
echo -e "${BOLD}Results: $TOTAL tests   ${GREEN}$PASS passed${NC}  ${RED}$FAIL failed${NC}  ${YELLOW}$SKIP skipped${NC}${NC}"
echo -e "${BOLD}════════════════════════════════════════${NC}"

if [[ ${#FAILED_TESTS[@]} -gt 0 ]]; then
    echo -e "\n${RED}Failed tests:${NC}"
    for t in "${FAILED_TESTS[@]}"; do echo -e "  ${RED}✗${NC} $t"; done
fi

if [[ $FAIL -eq 0 ]]; then
    echo -e "\n${GREEN}${BOLD}All sequences passed!${NC}"; exit 0
else
    echo -e "\n${RED}${BOLD}$FAIL sequence(s) failed.${NC}"; exit 1
fi
