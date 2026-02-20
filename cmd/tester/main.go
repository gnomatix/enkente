package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gnomatix/enkente/pkg/parser"
)

func main() {
	var logFile string
	flag.StringVar(&logFile, "log", "", "Path to the live logs.json to tail")
	flag.Parse()

	if logFile == "" {
		log.Fatal("Please provide a path to the live logs.json using -log")
	}

	fmt.Printf("Starting enkente live tailer on %s\n", logFile)
	fmt.Println("Waiting for messages... (Ctrl+C to quit)")

	done := make(chan struct{})

	// Setup signal handling for clean shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		fmt.Println("\nShutting down tailer...")
		close(done)
	}()

	// ANSI Color Codes
	colorReset := "\033[0m"
	colorWorker := "\033[36m" // Cyan
	colorSystem := "\033[34m" // Blue
	colorUser := "\033[32m"   // Green
	colorTime := "\033[90m"   // Dark Gray

	// The handler prints incoming messages to the console with ANSI colors indicating the worker
	handler := func(workerID int, msg parser.AntigravityMessage) {
		typeColor := colorSystem
		if msg.Type == "user" {
			typeColor = colorUser
		}

		fmt.Printf("%s[%s]%s %s[Worker-%d]%s %s%s: %s%s\n",
			colorTime, msg.Timestamp.Format("15:04:05"), colorReset,
			colorWorker, workerID, colorReset,
			typeColor, msg.Type, msg.Message, colorReset)
	}

	// Spin up the tailer with 4 worker coroutines to see the swarm in action
	err := parser.TailChatLog(logFile, 500*time.Millisecond, 4, handler, done)
	if err != nil {
		log.Fatalf("Failed to start tailer: %v", err)
	}

	// Block main until shut down
	<-done
	fmt.Println("Goodbye.")
}
