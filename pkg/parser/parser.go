package parser

import (
	"encoding/json"
	"io"
	"os"
	"time"
)

// AntigravityMessage represents a single chat turn in the Antigravity logs.
type AntigravityMessage struct {
	SessionID string    `json:"sessionId"`
	MessageID int       `json:"messageId"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// ParseChatLog reads the provided Antigravity JSON log file and unmarshals it.
func ParseChatLog(filePath string) ([]AntigravityMessage, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var messages []AntigravityMessage
	err = json.Unmarshal(bytes, &messages)
	if err != nil {
		return nil, err
	}

	return messages, nil
}
