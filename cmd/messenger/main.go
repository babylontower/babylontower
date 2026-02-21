package main

import (
	"fmt"
)

// Version information injected at build time
var (
	Version   = "unknown"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	fmt.Printf("Babylon Tower v%s - Decentralized P2P Messenger\n", Version)
	fmt.Printf("Build: %s | Commit: %s\n", BuildTime, GitCommit)
	fmt.Println("Initial development phase - PoC coming soon")
}
