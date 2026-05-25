package main

import (
	"regexp"
	"strings"
)

// parseArgs parses an argument list from inside parentheses.
func parseArgs(argsStr string) []sdlArg {
	argsStr = strings.TrimSpace(argsStr)
	if argsStr == "" {
		return nil
	}

	if strings.Contains(argsStr, ",") {
		parts := splitArgs(argsStr)
		var args []sdlArg
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			arg := parseOneArg(part)
			if arg != nil {
				args = append(args, *arg)
			}
		}
		if len(args) > 0 {
			return args
		}
	}

	return parseArgsRegex(argsStr)
}

var sdlArgFinder = regexp.MustCompile(`(?:"""((?:[^"]|"[^"]|""[^"])*)"""\s+)?(\w+)\s*:\s*(\[?\w+!?\]?!?)`)

// parseArgsRegex extracts all arguments from a string using regex matching.
func parseArgsRegex(argsStr string) []sdlArg {
	matches := sdlArgFinder.FindAllStringSubmatch(argsStr, -1)
	var args []sdlArg

	for _, match := range matches {
		desc := strings.TrimSpace(match[1])
		desc = strings.Join(strings.Fields(desc), " ")
		name := match[2]
		typeStr := match[3]
		required := strings.HasSuffix(typeStr, "!")

		args = append(args, sdlArg{
			Name:        name,
			Type:        typeStr,
			Description: desc,
			Required:    required,
		})
	}

	return args
}

// splitArgs splits an args string on commas, respecting nested brackets.
func splitArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, ch := range s {
		switch ch {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

var argRe = regexp.MustCompile(`^(?:"""[^"]*"""\s*)?(\w+)\s*:\s*([^\s=]+(?:\s*=\s*.+)?)$`)

// parseOneArg parses a single argument definition.
func parseOneArg(s string) *sdlArg {
	s = strings.TrimSpace(s)

	desc := ""
	if strings.HasPrefix(s, `"""`) {
		end := strings.Index(s[3:], `"""`)
		if end >= 0 {
			desc = strings.TrimSpace(s[3 : 3+end])
			s = strings.TrimSpace(s[3+end+3:])
		}
	} else if strings.HasPrefix(s, `"`) {
		end := strings.Index(s[1:], `"`)
		if end >= 0 {
			desc = strings.TrimSpace(s[1 : 1+end])
			s = strings.TrimSpace(s[1+end+1:])
		}
	}

	m := argRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}

	name := m[1]
	typeStr := m[2]

	if idx := strings.Index(typeStr, "="); idx >= 0 {
		typeStr = strings.TrimSpace(typeStr[:idx])
	}

	required := strings.HasSuffix(typeStr, "!")

	return &sdlArg{
		Name:        name,
		Type:        typeStr,
		Description: desc,
		Required:    required,
	}
}
