package parser

import (
	"os"
	"time"
)

// messageIdentity is the (session, id) pair that uniquely names a message.
type messageIdentity struct {
	sessionID string
	messageID int
}

func identityOf(msg AntigravityMessage) messageIdentity {
	return messageIdentity{sessionID: msg.SessionID, messageID: msg.MessageID}
}

// TailChatLog watches a given Antigravity JSON log file and streams new messages to the returned channel.
// It also spins up `numWorkers` goroutines to process incoming messages concurrently using the provided `handler`.
// It stops if the done channel is closed.
func TailChatLog(filePath string, pollInterval time.Duration, numWorkers int, handler func(workerID int, msg AntigravityMessage), done <-chan struct{}) error {
	out := make(chan AntigravityMessage, 100) // Buffer the channel to prevent blocking on fast writes

	// Worker swarm: start the specified number of goroutines
	for i := 0; i < numWorkers; i++ {
		workerID := i
		go func() {
			for msg := range out {
				handler(workerID, msg)
			}
		}()
	}

	// Tailing routine
	go func() {
		defer close(out)

		lastModTime := time.Time{}
		lastSize := int64(-1)
		lastProcessedIndex := 0
		// Identity of the last message handed out, used to notice when the
		// file has been replaced by a different log of the same length.
		var lastSeen messageIdentity

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

				// Re-read on either signal: coarse modtime granularity on some
				// filesystems can hide a rewrite that happened within the same
				// tick, while a size change never lies.
				if !info.ModTime().After(lastModTime) && info.Size() == lastSize {
					continue
				}
				lastModTime = info.ModTime()
				lastSize = info.Size()

				messages, err := ParseChatLog(filePath)
				if err != nil {
					// Malformed JSON (perhaps caught mid-write), we'll try again next tick
					continue
				}

				// Treat the file as rotated when it shrank or when the message
				// at our high-water mark is no longer the one we last emitted
				// (a different session's log written over this path).
				rotated := len(messages) < lastProcessedIndex ||
					(lastProcessedIndex > 0 && identityOf(messages[lastProcessedIndex-1]) != lastSeen)
				if rotated {
					lastProcessedIndex = 0
				}

				if len(messages) > lastProcessedIndex {
					for _, msg := range messages[lastProcessedIndex:] {
						out <- msg
					}
					lastProcessedIndex = len(messages)
					lastSeen = identityOf(messages[lastProcessedIndex-1])
				}
			}
		}
	}()

	return nil
}
