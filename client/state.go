package main

type ClientState struct {
	UserID         string
	ListenAddr     string
	Files          map[string]string // filename -> filepath
	TrackerAddrs   []string          // All configured tracker addresses
	ActiveTrackers []string          // Currently responsive trackers
}

var State = &ClientState{
	TrackerAddrs:   []string{},
	ActiveTrackers: []string{},
	Files:          make(map[string]string),
}
