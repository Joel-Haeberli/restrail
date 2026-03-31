package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"restrail/internal/tmpl"
	"strings"
)

// WriteRequest writes a .request file to disk. The file is a Go text/template
// that will be executed at runtime by the plan runner.
//
// Path parameters like {orderId} are converted to {{ param "/path/{orderId}" }}.
// FK sentinel values in the body are converted to {{ fk "domain" }} expressions.
func WriteRequest(dir, filename, method, path string, headers map[string]string, body map[string]interface{}) error {
	var b strings.Builder

	// Request line — convert path params to template expression
	templatePath := tmpl.ConvertPathToTemplate(path)
	fmt.Fprintf(&b, "%s %s HTTP/1.1\n", method, templatePath)

	// Headers
	for key, val := range headers {
		fmt.Fprintf(&b, "%s: %s\n", key, val)
	}

	// Body
	if body != nil {
		b.WriteString("\n")
		jsonBytes, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling request body: %w", err)
		}
		// Convert FK sentinels to Go template expressions
		bodyStr := tmpl.ConvertSentinels(string(jsonBytes))
		b.WriteString(bodyStr)
		b.WriteString("\n")
	}

	return os.WriteFile(filepath.Join(dir, filename), []byte(b.String()), 0644)
}

// RequestFilename generates a filename for a request file.
// crudSuffix helps disambiguate GET operations (LIST vs BY_ID).
func RequestFilename(domain, method, crudSuffix string) string {
	if crudSuffix != "" {
		return fmt.Sprintf("%s_%s_%s.request", domain, method, crudSuffix)
	}
	return fmt.Sprintf("%s_%s.request", domain, method)
}
