package main

import (
	"net"
	"p2p/common"
	"time"
)

// SendToTracker tries active trackers first, then any remaining known trackers.
// Returns the first successful response. Fast failover — no re-scan.
func SendToTracker(msg Message) Response {
	// Build candidate list: active trackers first, then remaining known addresses
	seen := make(map[string]bool)
	candidates := make([]string, 0)
	for _, addr := range State.ActiveTrackers {
		candidates = append(candidates, addr)
		seen[addr] = true
	}
	for _, addr := range State.TrackerAddrs {
		if !seen[addr] {
			candidates = append(candidates, addr)
		}
	}

	for _, addr := range candidates {
		resp, ok := tryTracker(addr, msg)
		if ok {
			return resp
		}
	}

	return Response{"error", "no trackers available"}
}

// BroadcastToTrackers sends message to all active trackers (for state changes)
func BroadcastToTrackers(msg Message) []Response {
	responses := make([]Response, 0)
	responseChan := make(chan Response, len(State.ActiveTrackers))
	
	for _, addr := range State.ActiveTrackers {
		go func(address string) {
			resp, ok := tryTracker(address, msg)
			if ok {
				responseChan <- resp
			}
		}(addr)
	}
	
	// Collect responses with timeout
	timeout := time.After(2 * time.Second)
	for i := 0; i < len(State.ActiveTrackers); i++ {
		select {
		case resp := <-responseChan:
			responses = append(responses, resp)
		case <-timeout:
			goto done
		}
	}
done:
	return responses
}

// tryTracker attempts to send message to a single tracker
func tryTracker(addr string, msg Message) (Response, bool) {
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return Response{}, false
	}
	defer conn.Close()
	
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	
	if err := common.Send(conn, msg); err != nil {
		return Response{}, false
	}
	
	var resp Response
	if err := common.Recv(conn, &resp); err != nil {
		return Response{}, false
	}
	
	return resp, true
}

// UpdateActiveTrackers checks which trackers are responsive
func UpdateActiveTrackers() {
	active := make([]string, 0)
	
	for _, addr := range State.TrackerAddrs {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			active = append(active, addr)
		}
	}
	
	State.ActiveTrackers = active
}
