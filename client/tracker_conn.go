package main

import (
	"net"
	"p2p/common"
	"time"
)

// SendToTracker tries trackers in order, returns first successful response
func SendToTracker(msg Message) Response {
	// Try active trackers first
	for _, addr := range State.ActiveTrackers {
		resp, ok := tryTracker(addr, msg)
		if ok {
			return resp
		}
	}
	
	// If all active trackers failed, refresh and try again
	UpdateActiveTrackers()
	for _, addr := range State.ActiveTrackers {
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
