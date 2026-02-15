package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

const SessionFile = ".p2p_session.json"

// SessionData stores persistent login state
type SessionData struct {
	UserID     string `json:"user_id"`
	ListenAddr string `json:"listen_addr"`
}

// LoadSession reads session from file and populates State
func LoadSession() error {
	data, err := os.ReadFile(SessionFile)
	if err != nil {
		// Session file doesn't exist or can't be read
		if os.IsNotExist(err) {
			return nil // Not an error, just no session
		}
		return err
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return err
	}

	// Populate global State
	State.UserID = session.UserID
	State.ListenAddr = session.ListenAddr

	return nil
}

// SaveSession writes current State to session file
func SaveSession() error {
	session := SessionData{
		UserID:     State.UserID,
		ListenAddr: State.ListenAddr,
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(SessionFile, data, 0600)
}

// ClearSession deletes the session file
func ClearSession() error {
	err := os.Remove(SessionFile)
	if os.IsNotExist(err) {
		return nil // Not an error if file doesn't exist
	}
	return err
}

// LoadTrackerConfig loads tracker addresses from config file
func LoadTrackerConfig(path string) {
	file, err := os.Open(path)
	if err != nil {
		// Default to localhost if config not found
		State.TrackerAddrs = []string{"127.0.0.1:9000"}
		State.ActiveTrackers = []string{"127.0.0.1:9000"}
		return
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line != "" && !strings.HasPrefix(line, "#") {
			State.TrackerAddrs = append(State.TrackerAddrs, line)
		}
	}
	
	// Update which trackers are currently active
	UpdateActiveTrackers()
}
