package model

import (
	"regexp"
	"strings"
)

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func GenerateSlug(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = slugNonAlnum.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "item"
	}
	return slug
}
