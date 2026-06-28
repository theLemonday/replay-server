package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type state struct {
	Servers []SubServer `json:"servers"`
}

// saveState writes all sub-servers to path atomically.
// It writes to a .tmp file first, then renames — so a mid-write crash never
// produces a corrupt state file.
func saveState(path string, servers []SubServer) error {
	data, err := json.MarshalIndent(state{Servers: servers}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

// loadState reads the state file. Returns an empty slice when the file does
// not yet exist (first run).
func loadState(path string) ([]SubServer, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return s.Servers, nil
}
