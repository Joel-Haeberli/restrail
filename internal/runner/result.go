package runner

import (
	"encoding/json"
	"restrail/internal/openapi"
	"time"
)

// DurationMs wraps time.Duration to serialize as milliseconds in JSON.
type DurationMs time.Duration

func (d DurationMs) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).Milliseconds())
}

func (d *DurationMs) UnmarshalJSON(b []byte) error {
	var ms int64
	if err := json.Unmarshal(b, &ms); err != nil {
		return err
	}
	*d = DurationMs(time.Duration(ms) * time.Millisecond)
	return nil
}

type OperationResult struct {
	OperationName    string                  `json:"operation_name"`
	Description      string                  `json:"description"`
	Method           string                  `json:"method"`
	Path             string                  `json:"path"`
	AuthApplied      string                  `json:"auth_applied"`
	AuthToken        string                  `json:"auth_token,omitempty"`
	SecurityRequired []openapi.SecurityInfo  `json:"security_required,omitempty"`
	RequestBody      interface{}             `json:"request_body,omitempty"`
	ExpectedStatus   int                     `json:"expected_status"`
	ActualStatus     int                     `json:"actual_status"`
	ResponseBody     interface{}             `json:"response_body,omitempty"`
	Success          bool                    `json:"success"`
	Skipped          bool                    `json:"skipped"`
	SkipReason       string                  `json:"skip_reason,omitempty"`
	Error            string                  `json:"error,omitempty"`
	Duration         DurationMs              `json:"duration_ms"`
}

// SetupOperation records a prerequisite operation from a dependency domain.
type SetupOperation struct {
	Domain    string          `json:"domain"`
	Operation OperationResult `json:"operation"`
}

type DomainResult struct {
	Domain     string            `json:"domain"`
	Setup      []SetupOperation  `json:"setup,omitempty"`
	Operations []OperationResult `json:"operations"`
	Success    bool              `json:"success"`
}

// DependencyInfo describes a detected FK relationship for reporting.
type DependencyInfo struct {
	FromDomain string `json:"from_domain"`
	ToDomain   string `json:"to_domain"`
	FieldName  string `json:"field_name"`
	Required   bool   `json:"required"`
	Resolved   bool   `json:"resolved"`
}

// CreatedResource records a resource created during the run.
type CreatedResource struct {
	Domain  string `json:"domain"`
	ID      string `json:"id"`
	IDField string `json:"id_field"`
}

type RunResult struct {
	Timestamp        time.Time         `json:"timestamp"`
	SpecFile         string            `json:"spec_file"`
	BaseURL          string            `json:"base_url"`
	Profile          string            `json:"profile"`
	AuthType         string            `json:"auth_type"`
	Dependencies     []DependencyInfo  `json:"dependencies,omitempty"`
	CreatedResources []CreatedResource `json:"created_resources,omitempty"`
	Domains          []DomainResult    `json:"domains"`
	Summary          Summary           `json:"summary"`
}

type Summary struct {
	TotalDomains  int `json:"total_domains"`
	PassedDomains int `json:"passed_domains"`
	FailedDomains int `json:"failed_domains"`
	TotalOps      int `json:"total_operations"`
	PassedOps     int `json:"passed_operations"`
	FailedOps     int `json:"failed_operations"`
	SkippedOps    int `json:"skipped_operations"`
}
