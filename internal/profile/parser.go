package profile

import (
	"fmt"
	"strconv"
	"strings"
)

func Parse(name, content string) (*Profile, error) {
	sections := splitSections(content)

	defSection, ok := sections["Definition"]
	if !ok {
		return nil, fmt.Errorf("profile %q: missing Definition section", name)
	}
	execSection, ok := sections["Execution"]
	if !ok {
		return nil, fmt.Errorf("profile %q: missing Execution section", name)
	}

	ops, err := parseOperations(defSection)
	if err != nil {
		return nil, fmt.Errorf("profile %q: %w", name, err)
	}

	order, err := parseExecutionOrder(execSection)
	if err != nil {
		return nil, fmt.Errorf("profile %q: %w", name, err)
	}

	var paramMappings map[string]string
	if paramsSection, ok := sections["Params"]; ok {
		paramMappings, err = parseParamMappings(paramsSection)
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", name, err)
		}
	}

	return &Profile{
		Name:           name,
		Operations:     ops,
		ExecutionOrder: order,
		ParamMappings:  paramMappings,
	}, nil
}

func splitSections(content string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(content, "\n")
	var currentSection string
	var sectionLines []string

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "-----------" {
			// Save previous section
			if currentSection != "" {
				sections[currentSection] = strings.TrimSpace(strings.Join(sectionLines, "\n"))
			}
			// Next non-separator line is the section name
			i++
			if i < len(lines) {
				currentSection = strings.TrimSpace(lines[i])
				sectionLines = nil
				// Skip the closing separator
				i++
			}
			continue
		}
		if currentSection != "" {
			sectionLines = append(sectionLines, lines[i])
		}
	}
	if currentSection != "" {
		sections[currentSection] = strings.TrimSpace(strings.Join(sectionLines, "\n"))
	}
	return sections
}

func parseOperations(section string) ([]Operation, error) {
	var ops []Operation
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		op, err := parseOperationLine(line)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func parseOperationLine(line string) (Operation, error) {
	var op Operation

	// Check optional: [OPn]: ...
	if strings.HasPrefix(line, "[") {
		closeBracket := strings.Index(line, "]")
		if closeBracket == -1 {
			return op, fmt.Errorf("malformed optional operation: %s", line)
		}
		op.Name = line[1:closeBracket]
		op.Optional = true
		line = strings.TrimSpace(line[closeBracket+1:])
		if !strings.HasPrefix(line, ":") {
			return op, fmt.Errorf("expected ':' after operation name: %s", line)
		}
		line = strings.TrimSpace(line[1:])
	} else {
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			return op, fmt.Errorf("expected ':' in operation line: %s", line)
		}
		op.Name = strings.TrimSpace(line[:colonIdx])
		line = strings.TrimSpace(line[colonIdx+1:])
	}

	// Parse: METHOD 'PATTERN' SEND_TYPE READ_TYPE STATUS: DESCRIPTION [PRECONDITION-DATA]
	// Extract method
	spaceIdx := strings.Index(line, " ")
	if spaceIdx == -1 {
		return op, fmt.Errorf("expected method in: %s", line)
	}
	op.Method = strings.ToUpper(strings.TrimSpace(line[:spaceIdx]))
	line = strings.TrimSpace(line[spaceIdx+1:])

	// Extract pattern (in single quotes)
	if !strings.HasPrefix(line, "'") {
		return op, fmt.Errorf("expected quoted pattern in: %s", line)
	}
	endQuote := strings.Index(line[1:], "'")
	if endQuote == -1 {
		return op, fmt.Errorf("unterminated pattern quote in: %s", line)
	}
	op.Pattern = line[1 : endQuote+1]
	line = strings.TrimSpace(line[endQuote+2:])

	// Extract SEND_TYPE
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return op, fmt.Errorf("expected SEND_TYPE READ_TYPE STATUS: ... in: %s", line)
	}
	op.SendType = CRUDType(parts[0])
	op.ReadType = CRUDType(parts[1])

	// Extract status (before the colon)
	statusStr := parts[2]
	statusStr = strings.TrimSuffix(statusStr, ":")
	status, err := strconv.Atoi(statusStr)
	if err != nil {
		return op, fmt.Errorf("invalid status code %q: %w", statusStr, err)
	}
	op.ExpectedStatus = status

	// Remaining is description, possibly with [PRECONDITION-DATA]
	descStart := strings.Index(line, ":")
	if descStart == -1 {
		// Status already had colon trimmed, find it in remaining text
		op.Description = strings.Join(parts[3:], " ")
	} else {
		remaining := strings.TrimSpace(line[descStart+1:])
		op.Description = remaining
	}

	// Check for [PRECONDITION-DATA]
	if strings.Contains(op.Description, "[PRECONDITION-DATA]") {
		op.Precondition = true
		op.Description = strings.TrimSpace(strings.Replace(op.Description, "[PRECONDITION-DATA]", "", 1))
	}

	return op, nil
}

func parseExecutionOrder(section string) ([]string, error) {
	section = strings.TrimSpace(section)
	if section == "" {
		return nil, fmt.Errorf("empty execution section")
	}
	parts := strings.Split(section, "->")
	var order []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			order = append(order, p)
		}
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("no operations in execution order")
	}
	return order, nil
}

func parseParamMappings(section string) (map[string]string, error) {
	mappings := make(map[string]string)
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid param mapping: %q (expected PARAM = DOMAIN)", line)
		}
		param := strings.TrimSpace(parts[0])
		domain := strings.TrimSpace(parts[1])
		if param == "" {
			return nil, fmt.Errorf("empty param name in mapping: %q", line)
		}
		if domain == "" {
			return nil, fmt.Errorf("empty domain in mapping: %q", line)
		}
		mappings[param] = domain
	}
	return mappings, nil
}

// OperationByName returns the operation with the given name, or nil.
func (p *Profile) OperationByName(name string) *Operation {
	for i := range p.Operations {
		if p.Operations[i].Name == name {
			return &p.Operations[i]
		}
	}
	return nil
}
