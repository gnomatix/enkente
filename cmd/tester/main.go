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

	// The handler simply prints incoming messages to the console
	handler := func(msg parser.AntigravityMessage) {
		fmt.Printf("[%s] %s: %s\n", msg.Timestamp.Format(time.Kitchen), msg.Type, msg.Message)
	}

	// Spin up the tailer with 2 worker coroutines
	err := parser.TailChatLog(logFile, 500*time.Millisecond, 2, handler, done)
	if err != nil {
		log.Fatalf("Failed to start tailer: %v", err)
	}

	// Block main until shut down
	<-done
	fmt.Println("Goodbye.")
}
