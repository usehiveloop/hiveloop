package docker

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func envList(input map[string]string) []string {
	if len(input) == 0 {
		return nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+input[key])
	}
	return env
}

func imageTagName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '.', r == '_':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r), r == '-':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		return "template"
	}
	return out
}

func containerName(name string) string {
	name = imageTagName(name)
	if name == "template" {
		name = "sandbox"
	}
	return name + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
