package main

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

var sizes = model.TemplateSizes

func resolveSizes(s string) ([]string, error) {
	if s == "all" {
		return []string{"small", "medium", "large", "xlarge"}, nil
	}
	out := []string{}
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if _, ok := sizes[name]; !ok {
			return nil, fmt.Errorf("unknown size %q (valid: small, medium, large, xlarge, all)", name)
		}
		out = append(out, name)
	}
	return out, nil
}
