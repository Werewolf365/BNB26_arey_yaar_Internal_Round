package runner

import (
	"encoding/json"
	"fmt"
	"os"
)

// BuilderRegistration is one entry in the local builder registry.
type BuilderRegistration struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName,omitempty"`
	IndependenceGroup string `json:"independenceGroup"`
	Endpoint         string `json:"endpoint,omitempty"`
}

// LoadRegistry reads the registry file (missing file = empty registry).
func LoadRegistry(path string) ([]BuilderRegistration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("OPERATIONAL: cannot read registry: %v", err)
	}
	var regs []BuilderRegistration
	if err := json.Unmarshal(data, &regs); err != nil {
		return nil, fmt.Errorf("INVALID_INPUT: malformed registry JSON: %v", err)
	}
	return regs, nil
}

// Register adds a builder; duplicates and empty identity fields are rejected.
func Register(path string, b BuilderRegistration) error {
	if b.ID == "" {
		return fmt.Errorf("INVALID_INPUT: --id is required")
	}
	if b.IndependenceGroup == "" {
		return fmt.Errorf("INVALID_INPUT: --group is required (independence_group must be explicit)")
	}
	regs, err := LoadRegistry(path)
	if err != nil {
		return err
	}
	for _, r := range regs {
		if r.ID == b.ID {
			return fmt.Errorf("INVALID_INPUT: builder %q already registered", b.ID)
		}
	}
	regs = append(regs, b)
	data, err := json.MarshalIndent(regs, "", "  ")
	if err != nil {
		return fmt.Errorf("OPERATIONAL: cannot encode registry: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("OPERATIONAL: cannot write registry: %v", err)
	}
	return nil
}
