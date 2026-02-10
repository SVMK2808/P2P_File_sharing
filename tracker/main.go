package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

func main() {
	// Default address
	address := ":9000"

	// Check for command-line arguments
	if len(os.Args) == 3 {
		configFile := os.Args[1]
		lineNum, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Printf("Error: Invalid line number '%s'\n", os.Args[2])
			fmt.Println("Usage: ./tracker_bin <config_file> <line_number>")
			os.Exit(1)
		}

		// Read config file
		file, err := os.Open(configFile)
		if err != nil {
			fmt.Printf("Error: Cannot open config file '%s': %v\n", configFile, err)
			os.Exit(1)
		}
		defer file.Close()

		// Read lines
		scanner := bufio.NewScanner(file)
		lines := []string{}
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				lines = append(lines, line)
			}
		}

		// Validate line number
		if lineNum < 1 || lineNum > len(lines) {
			fmt.Printf("Error: Line number %d out of range (1-%d)\n", lineNum, len(lines))
			os.Exit(1)
		}

		address = lines[lineNum-1]
		fmt.Printf("Using tracker address from config: %s\n", address)
	} else if len(os.Args) == 1 {
		fmt.Printf("Using default address: %s\n", address)
	} else {
		fmt.Println("Usage: ./tracker_bin [config_file] [line_number]")
		fmt.Println("Example: ./tracker_bin tracker_info.txt 1")
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Printf("Error: Failed to start tracker on %s: %v\n", address, err)
		os.Exit(1)
	}

	fmt.Printf("Tracker listening on %s\n", address)
	fmt.Println("Type 'quit' to stop the tracker")

	quit := make(chan bool)

	// Listen for quit command in background
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if scanner.Text() == "quit" {
				fmt.Println("Tracker shutting down...")
				ln.Close() // Close listener to unblock Accept()
				quit <- true
				return
			}
		}
	}()

	// Accept connections in a goroutine
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Listener was closed, exit gracefully
				return
			}
			go handleConn(conn)
		}
	}()

	<-quit
	fmt.Println("Tracker stopped.")
}
