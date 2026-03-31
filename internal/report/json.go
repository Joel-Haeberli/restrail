package report

import (
	"encoding/json"
	"restrail/internal/runner"
)

type JSONReporter struct{}

func (r *JSONReporter) Generate(result *runner.RunResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

func (r *JSONReporter) Extension() string {
	return ".json"
}
