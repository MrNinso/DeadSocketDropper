package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Constants and Global Variables
var (
	sourcePort        string
	checkIntervalMin  int
	maxActiveDurMin   int
	maxInactiveDurMin int
	
	connections = make(map[string]*ConnectionInfo)
	mu          sync.Mutex
	inodeRegex = regexp.MustCompile(`ino:([0-9]+)`)
)

// ConnectionInfo stores the state of a tracked connection
type ConnectionInfo struct {
	Inode        string    
	TimeAdded    time.Time
	LastSeen     time.Time
	IsActive     bool      
	ConnectionID string    
}

func init() {
	// Configure command-line flags and help messages in English
	flag.StringVar(&sourcePort, "port", "50090", "Source port to be monitored (e.g., 50090)")
	flag.IntVar(&checkIntervalMin, "check-interval", 30, "Check interval in minutes (e.g., 30)")
	flag.IntVar(&maxActiveDurMin, "max-active", 120, "Maximum allowed active duration in minutes (e.g., 120 for 2h)")
	flag.IntVar(&maxInactiveDurMin, "max-inactive", 60, "Time unused before being removed from list, in minutes (e.g., 60 for 1h)")

	// Define a custom usage function for clear help output
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args)
		fmt.Fprintf(os.Stderr, "Monitors, tracks, and kills TCP connections on a specific source port.\n\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nNOTE: This program MUST be run as root (sudo).\n")
	}
}

func main() {
	flag.Parse()

	// 1. Check environment (Linux, ss in PATH, root UID)
	if err := checkEnvironment(); err != nil {
		log.Fatalf("Environment error: %v", err)
	}

	fmt.Printf("Monitoring started on port: %s\n", sourcePort)
	fmt.Printf("Check Interval: %d min\n", checkIntervalMin)
	fmt.Printf("Max Active Duration: %d min\n", maxActiveDurMin)
	fmt.Printf("Max Inactive Duration: %d min\n", maxInactiveDurMin)

	// Start the loop immediately and then every interval
	ticker := time.NewTicker(time.Duration(checkIntervalMin) * time.Minute)
	defer ticker.Stop()

	for {
		monitorConnections()
		<-ticker.C
	}
}

// checkEnvironment validates the OS and privileges
func checkEnvironment() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("this script only works on Linux. Current OS: %s", runtime.GOOS)
	}

	// Check if 'ss' is in PATH
	if _, err := exec.LookPath("ss"); err != nil {
		return fmt.Errorf("ss utility not found in PATH. Install iproute2 package")
	}

	// Check if user is root
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("could not determine current user: %w", err)
	}

	// UID 0 is root on Linux
	if currentUser.Uid != "0" {
		return fmt.Errorf("this program must be run as root (sudo). Current UID: %s", currentUser.Uid)
	}

	return nil
}

func monitorConnections() {
	mu.Lock()
	defer mu.Unlock()

	fmt.Println("\n--- Executing monitoring cycle:", time.Now().Format(time.RFC1123), "---")

	currentConnsList, err := listCurrentConnections()
	if err != nil {
		log.Printf("Error listing connections: %v", err)
		return
	}

	for _, conn := range connections {
		conn.IsActive = false
	}

	now := time.Now()
	for _, currentConn := range currentConnsList {
		if connInfo, exists := connections[currentConn.Inode]; exists {
			connInfo.LastSeen = now
			connInfo.IsActive = true
		} else {
			connections[currentConn.Inode] = currentConn
			currentConn.TimeAdded = now
			currentConn.LastSeen = now
			fmt.Printf(" + New connection tracked (Inode %s): %s\n", currentConn.Inode, currentConn.ConnectionID)
		}
	}

	// 3. Process connections to kill or remove
	for inode, conn := range connections {
		// A. Kill active connections older than maxActiveDurMin
		maxActiveDuration := time.Duration(maxActiveDurMin) * time.Minute
		if now.Sub(conn.TimeAdded) > maxActiveDuration && conn.IsActive {
			fmt.Printf(" x Killing active connection (>%d min, Inode %s): %s\n", maxActiveDurMin, inode, conn.ConnectionID)
			killConnection(inode) 
			delete(connections, inode) 
			continue
		}

		// B. Remove connections inactive for longer than maxInactiveDurMin
		maxInactiveDuration := time.Duration(maxInactiveDurMin) * time.Minute
		if now.Sub(conn.LastSeen) > maxInactiveDuration {
			fmt.Printf(" - Removing inactive connection (>%d min, Inode %s): %s\n", maxInactiveDurMin, inode, conn.ConnectionID)
			delete(connections, inode)
			continue
		}
	}

	fmt.Printf("Total tracked connections: %d\n", len(connections))
}


func listCurrentConnections() ([]*ConnectionInfo, error) {
	cmd := exec.Command("ss", "-tnpeH", "src", ":"+sourcePort)
	stdout, err := cmd.StdoutPipe()

	if err != nil {
		return nil, fmt.Errorf("StdoutPipe error: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cmd Start error: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	var currentConnections []*ConnectionInfo

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		matches := inodeRegex.FindStringSubmatch(line)
		if len(matches) < 2 {
			log.Printf("Warning: Could not extract inode from line: %s", line)
			continue
		}
		inode := matches[1] // Use index 1 for the captured group

		if len(fields) >= 5 {
			// Extracting local and peer addresses from fields
            // Assuming fields[3] is local address and fields[4] is peer address based on typical ss output
			localAddr := fields[3] 
			peerAddr := fields[4] 
			connID := fmt.Sprintf("%s -> %s", localAddr, peerAddr)

			currentConnections = append(currentConnections, &ConnectionInfo{
				Inode:        inode,
				ConnectionID: connID,
				IsActive:     true,
			})
		}
	}

	cmd.Wait()

	return currentConnections, nil
}


func killConnection(inode string) error {
	connInfo, exists := connections[inode]
	if !exists {
		return fmt.Errorf("connection info not found for inode %s", inode)
	}
	
	parts := strings.Split(connInfo.ConnectionID, " -> ")
	if len(parts) != 2 {
		log.Printf("Invalid ID format for killing: %s\n", connInfo.ConnectionID)
		return fmt.Errorf("invalid connection ID format")
	}

	localAddr := parts[0]
	peerAddr := parts[1]

	// We use 'ss --kill' with src/dst filters
	cmd := exec.Command("ss", "--kill", "dst", peerAddr, "src", localAddr)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error executing kill for %s (Inode %s): %v\nOutput: %s", connInfo.ConnectionID, inode, err, string(output))
		return err
	}

	fmt.Printf(" -> Kill command executed for %s (Inode %s)\n", connInfo.ConnectionID, inode)
	return nil
}
