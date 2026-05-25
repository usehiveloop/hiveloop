package main

import (
	"regexp"
	"strings"
)

// parseFieldsFromBlock parses individual field definitions from a type body.
func parseFieldsFromBlock(block string) []sdlField {
	var fields []sdlField
	lines := strings.Split(block, "\n")

	var currentDesc strings.Builder
	var fieldAccum strings.Builder
	inBlockComment := false
	parenDepth := 0

	flushField := func() {
		if fieldAccum.Len() == 0 {
			return
		}
		fieldStr := strings.TrimSpace(fieldAccum.String())
		fieldAccum.Reset()

		field := parseFieldLine(fieldStr, strings.TrimSpace(currentDesc.String()))
		currentDesc.Reset()
		if field != nil {
			fields = append(fields, *field)
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" && parenDepth == 0 {
			continue
		}

		if strings.HasPrefix(trimmed, `"""`) && parenDepth == 0 {
			if inBlockComment {
				inBlockComment = false
				continue
			}
			if strings.Count(trimmed, `"""`) >= 2 {
				desc := strings.TrimPrefix(trimmed, `"""`)
				desc = strings.TrimSuffix(desc, `"""`)
				currentDesc.Reset()
				currentDesc.WriteString(strings.TrimSpace(desc))
				continue
			}
			inBlockComment = true
			currentDesc.Reset()
			continue
		}
		if inBlockComment {
			currentDesc.WriteString(trimmed)
			currentDesc.WriteString(" ")
			continue
		}

		if strings.HasPrefix(trimmed, `"`) && !strings.HasPrefix(trimmed, `""`) && parenDepth == 0 {
			desc := strings.Trim(trimmed, `"`)
			currentDesc.Reset()
			currentDesc.WriteString(desc)
			continue
		}

		if strings.HasPrefix(trimmed, "#") && parenDepth == 0 {
			continue
		}

		if fieldAccum.Len() > 0 {
			fieldAccum.WriteString(" ")
		}
		fieldAccum.WriteString(trimmed)

		for _, ch := range trimmed {
			switch ch {
			case '(':
				parenDepth++
			case ')':
				parenDepth--
			}
		}

		if parenDepth <= 0 {
			parenDepth = 0
			flushField()
		}
	}

	flushField()

	return fields
}

var fieldLineRe = regexp.MustCompile(`^(\w+)\s*(?:\(([^)]*)\))?\s*:\s*(.+?)(?:\s*@.*)?$`)

// parseFieldLine parses a single field definition line.
func parseFieldLine(line, description string) *sdlField {
	line = strings.TrimSpace(line)

	m := fieldLineRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}

	field := &sdlField{
		Name:        m[1],
		Description: strings.TrimSpace(description),
		Type:        strings.TrimSpace(m[3]),
	}

	if m[2] != "" {
		field.Args = parseArgs(m[2])
	}

	return field
}
