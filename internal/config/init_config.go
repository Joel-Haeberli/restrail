package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type InitConfig struct {
	SpecPath          string `json:"spec"`
	Profile           string `json:"profile"`
	OutputDir         string `json:"output_dir"`
	OptimisticLocking bool   `json:"optimistic_locking"`
}

func LoadInitConfig(path string) (InitConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return InitConfig{}, fmt.Errorf("reading init config file: %w", err)
	}

	var cfg InitConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return InitConfig{}, fmt.Errorf("parsing init config file: %w", err)
	}

	// Env vars override file values
	if v := os.Getenv("RESTRAIL_SPEC"); v != "" {
		cfg.SpecPath = v
	}
	if v := os.Getenv("RESTRAIL_PROFILE"); v != "" {
		cfg.Profile = v
	}
	if v := os.Getenv("RESTRAIL_OUTPUT_DIR"); v != "" {
		cfg.OutputDir = v
	}
	if v := os.Getenv("RESTRAIL_OPTIMISTIC_LOCK"); v != "" {
		cfg.OptimisticLocking = parseBool(v)
	}

	return cfg, nil
}

func LoadInitConfigFromEnv() InitConfig {
	return InitConfig{
		SpecPath:          os.Getenv("RESTRAIL_SPEC"),
		Profile:           os.Getenv("RESTRAIL_PROFILE"),
		OutputDir:         os.Getenv("RESTRAIL_OUTPUT_DIR"),
		OptimisticLocking: parseBool(os.Getenv("RESTRAIL_OPTIMISTIC_LOCK")),
	}
}

func (c InitConfig) Validate() error {
	if c.SpecPath == "" {
		return fmt.Errorf("spec is required")
	}
	if c.Profile == "" {
		return fmt.Errorf("profile is required")
	}
	return nil
}

func (c InitConfig) ResolvedOutputDir() string {
	if c.OutputDir != "" {
		return c.OutputDir
	}
	return "restrail-tests"
}
