package model

// TemplateSize defines the resource allocation for a sandbox template variant.
type TemplateSize struct {
	Name   string
	CPU    int
	Memory int // GB
	Disk   int // GB
}

// TemplateSizes maps size names to their resource allocations.
var TemplateSizes = map[string]TemplateSize{
	"small":  {Name: "small", CPU: 1, Memory: 2, Disk: 10},
	"medium": {Name: "medium", CPU: 2, Memory: 4, Disk: 20},
	"large":  {Name: "large", CPU: 4, Memory: 8, Disk: 40},
	"xlarge": {Name: "xlarge", CPU: 8, Memory: 16, Disk: 80},
}

// ValidTemplateSize returns true if the given size name is a valid template size.
func ValidTemplateSize(size string) bool {
	_, ok := TemplateSizes[size]
	return ok
}
