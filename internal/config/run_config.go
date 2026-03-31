package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type RunConfig struct {
	BaseURL           string   `json:"base_url"`
	CredsSubject      string   `json:"creds_subject"`
	CredsSecret       string   `json:"creds_secret"`
	CredsClientID     string   `json:"creds_client_id"`
	CredsClientSecret string   `json:"creds_client_secret"`
	OutputPath        string   `json:"output"`
	TestDir           string   `json:"test_dir"`
	OptimisticLocking bool     `json:"optimistic_locking"`
	AuthType          string   `json:"auth_type"`
	TokenURL          string   `json:"token_url,omitempty"`
	Scopes            []string `json:"scopes,omitempty"`
	SpecPath          string   `json:"spec,omitempty"`
	BlockedDomains    []string `json:"blocked_domains,omitempty"`
}

func LoadRunConfig(path string) (RunConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RunConfig{}, fmt.Errorf("reading run config file: %w", err)
	}

	var cfg RunConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return RunConfig{}, fmt.Errorf("parsing run config file: %w", err)
	}

	// Env vars override file values
	if v := os.Getenv("RESTRAIL_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("RESTRAIL_CREDS_SUBJECT"); v != "" {
		cfg.CredsSubject = v
	}
	if v := os.Getenv("RESTRAIL_CREDS_SECRET"); v != "" {
		cfg.CredsSecret = v
	}
	if v := os.Getenv("RESTRAIL_CREDS_CLIENT_ID"); v != "" {
		cfg.CredsClientID = v
	}
	if v := os.Getenv("RESTRAIL_CREDS_CLIENT_SECRET"); v != "" {
		cfg.CredsClientSecret = v
	}
	if v := os.Getenv("RESTRAIL_OUTPUT"); v != "" {
		cfg.OutputPath = v
	}
	if v := os.Getenv("RESTRAIL_OPTIMISTIC_LOCK"); v != "" {
		cfg.OptimisticLocking = parseBool(v)
	}
	if v := os.Getenv("RESTRAIL_BLOCKED_DOMAINS"); v != "" {
		cfg.BlockedDomains = splitCSV(v)
	}

	return cfg, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (c RunConfig) Validate() error {
	if c.BaseURL == "" {
		return fmt.Errorf("base_url is required")
	}
	if c.TestDir == "" {
		return fmt.Errorf("test_dir is required")
	}
	return nil
}
