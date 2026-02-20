package parser

import (
	"os"
	"time"
)

// TailChatLog watches a given Antigravity JSON log file and streams new messages to the returned channel.
// It also spins up `numWorkers` goroutines to process incoming messages concurrently using the provided `handler`.
// It stops if the done channel is closed.
func TailChatLog(filePath string, pollInterval time.Duration, numWorkers int, handler func(AntigravityMessage), done <-chan struct{}) error {
	out := make(chan AntigravityMessage, 100) // Buffer the channel to prevent blocking on fast writes

	// Worker swarm: start the specified number of goroutines
	for i := 0; i < numWorkers; i++ {
		go func() {
			for msg := range out {
				handler(msg)
			}
		}()
	}

	// Tailing routine
	go func() {
		defer close(out)

		lastModTime := time.Time{}
		lastProcessedIndex := 0

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				info, err := os.Stat(filePath)
				if err != nil {
					// File might not exist yet; ignore and continue polling
					continue
				}

				if info.ModTime().After(lastModTime) {
					lastModTime = info.ModTime()

					messages, err := ParseChatLog(filePath)
					if err != nil {
						// Malformed JSON (perhaps caught mid-write), we'll try again next tick
						continue
					}

					// If new messages were added
					if len(messages) > lastProcessedIndex {
						for _, msg := range messages[lastProcessedIndex:] {
							out <- msg
						}
						lastProcessedIndex = len(messages)
					} else if len(messages) < lastProcessedIndex {
						// File was truncated or rotated
						lastProcessedIndex = 0
						for _, msg := range messages {
							out <- msg
						}
						lastProcessedIndex = len(messages)
					}
				}
			}
		}
	}()

	return nil
}
