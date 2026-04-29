package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Event struct {
	At       time.Time `json:"at"`
	Action   string    `json:"action"`
	Service  string    `json:"service"`
	Mode     string    `json:"mode,omitempty"`
	URL      string    `json:"url,omitempty"`
	Provider string    `json:"provider,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type Log struct {
	path string
}

func NewLog(path string) Log {
	return Log{path: path}
}

func (l Log) Append(event Event) error {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(event)
}

func (l Log) ReadAll() ([]Event, error) {
	f, err := os.Open(l.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

func (l Log) Cleanup(retention time.Duration) error {
	events, err := l.ReadAll()
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-retention)
	var kept []Event
	for _, event := range events {
		if event.At.After(cutoff) || event.At.Equal(cutoff) {
			kept = append(kept, event)
		}
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, event := range kept {
		if err := enc.Encode(event); err != nil {
			return err
		}
	}
	return nil
}
