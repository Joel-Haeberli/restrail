package report

import (
	"fmt"
	"restrail/internal/openapi"
	"restrail/internal/runner"
	"strings"
	"time"
)

type MarkdownReporter struct{}

func (r *MarkdownReporter) Generate(result *runner.RunResult) ([]byte, error) {
	var b strings.Builder

	b.WriteString("# Restrail Test Report\n\n")
	b.WriteString(fmt.Sprintf("- **Timestamp**: %s\n", result.Timestamp.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- **Base URL**: %s\n", result.BaseURL))
	b.WriteString(fmt.Sprintf("- **Profile**: %s\n", result.Profile))
	b.WriteString(fmt.Sprintf("- **Auth**: %s\n", result.AuthType))
	if result.SpecFile != "" {
		b.WriteString(fmt.Sprintf("- **Spec**: %s\n", result.SpecFile))
	}
	b.WriteString("\n")

	// Summary
	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("| Metric | Count |\n"))
	b.WriteString(fmt.Sprintf("|--------|-------|\n"))
	b.WriteString(fmt.Sprintf("| Total Domains | %d |\n", result.Summary.TotalDomains))
	b.WriteString(fmt.Sprintf("| Passed Domains | %d |\n", result.Summary.PassedDomains))
	b.WriteString(fmt.Sprintf("| Failed Domains | %d |\n", result.Summary.FailedDomains))
	b.WriteString(fmt.Sprintf("| Total Operations | %d |\n", result.Summary.TotalOps))
	b.WriteString(fmt.Sprintf("| Passed Operations | %d |\n", result.Summary.PassedOps))
	b.WriteString(fmt.Sprintf("| Failed Operations | %d |\n", result.Summary.FailedOps))
	b.WriteString(fmt.Sprintf("| Skipped Operations | %d |\n", result.Summary.SkippedOps))
	b.WriteString("\n")

	// Created resources
	if len(result.CreatedResources) > 0 {
		b.WriteString("## Created Resources\n\n")
		b.WriteString("| # | Domain | ID Field | ID |\n")
		b.WriteString("|---|--------|----------|----|" + "\n")
		for i, cr := range result.CreatedResources {
			b.WriteString(fmt.Sprintf("| %d | %s | %s | %s |\n", i+1, cr.Domain, cr.IDField, cr.ID))
		}
		b.WriteString("\n")
	}

	// Per domain
	for _, dr := range result.Domains {
		status := "PASS"
		if !dr.Success {
			status = "FAIL"
		}
		b.WriteString(fmt.Sprintf("## Domain: %s [%s]\n\n", dr.Domain, status))

		// Setup operations
		if len(dr.Setup) > 0 {
			b.WriteString("### Setup (prerequisite operations)\n\n")
			b.WriteString("| Domain | Operation | Method | Path | Result |\n")
			b.WriteString("|--------|-----------|--------|------|--------|\n")
			for _, s := range dr.Setup {
				sResult := "PASS"
				if s.Operation.Skipped {
					sResult = "SKIP"
				} else if !s.Operation.Success {
					sResult = "FAIL"
				}
				b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
					s.Domain, s.Operation.OperationName, s.Operation.Method, s.Operation.Path, sResult))
			}
			b.WriteString("\n")
		}

		b.WriteString("| Operation | Method | Path | Auth Applied | Auth Token | Security Required | Expected | Actual | Result | Duration |\n")
		b.WriteString("|-----------|--------|------|--------------|------------|-------------------|----------|--------|--------|----------|\n")

		for _, op := range dr.Operations {
			result := "PASS"
			if op.Skipped {
				result = "SKIP"
			} else if !op.Success {
				result = "FAIL"
			}
			duration := fmt.Sprintf("%dms", time.Duration(op.Duration).Milliseconds())
			secRequired := formatSecurityMd(op.SecurityRequired)
			authToken := op.AuthToken
			if authToken == "" {
				authToken = "-"
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %d | %d | %s | %s |\n",
				op.OperationName, op.Method, op.Path,
				op.AuthApplied, truncateToken(authToken),
				secRequired,
				op.ExpectedStatus, op.ActualStatus, result, duration))
		}
		b.WriteString("\n")

		// Show errors
		for _, op := range dr.Operations {
			if op.Error != "" {
				b.WriteString(fmt.Sprintf("**%s Error**: %s\n\n", op.OperationName, op.Error))
			}
			if op.SkipReason != "" {
				b.WriteString(fmt.Sprintf("**%s Skipped**: %s\n\n", op.OperationName, op.SkipReason))
			}
		}
	}

	return []byte(b.String()), nil
}

func (r *MarkdownReporter) Extension() string {
	return ".md"
}

func truncateToken(token string) string {
	if len(token) <= 40 {
		return token
	}
	return token[:40] + "..."
}

func formatSecurityMd(infos []openapi.SecurityInfo) string {
	if len(infos) == 0 {
		return "none"
	}
	var parts []string
	for _, info := range infos {
		label := info.SchemeName + " (" + info.Type
		if info.Scheme != "" {
			label += "/" + info.Scheme
		}
		label += ")"
		if len(info.Scopes) > 0 {
			label += " [" + strings.Join(info.Scopes, ", ") + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ", ")
}
