package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const stateFile = "tracker_state.json"

// TrackerState represents all persistent state
type TrackerState struct {
	Users  map[string]*User  `json:"users"`
	Groups map[string]*Group `json:"groups"`
	Files  map[string]*File  `json:"files"`
}

// SaveState writes current state to disk
func SaveState() error {
	mu.Lock()
	defer mu.Unlock()
	
	state := TrackerState{
		Users:  users,
		Groups: groups,
		Files:  files,
	}
	
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(stateFile, data, 0644)
}

// LoadState reads state from disk if it exists
func LoadState() error {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// No saved state, start fresh
			fmt.Println("No saved state found, starting fresh")
			return nil
		}
		return err
	}
	
	var state TrackerState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	
	mu.Lock()
	defer mu.Unlock()
	
	if state.Users != nil {
		users = state.Users
		fmt.Printf("Loaded %d users from disk\n", len(users))
	}
	if state.Groups != nil {
		groups = state.Groups
		fmt.Printf("Loaded %d groups from disk\n", len(groups))
	}
	if state.Files != nil {
		files = state.Files
		fmt.Printf("Loaded %d files from disk\n", len(files))
	}
	
	return nil
}
