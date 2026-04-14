package model

// Symbol represents an extracted code symbol with metadata.
type Symbol struct {
	ID        int64
	RepoName  string
	Name      string
	FilePath  string // relative to repo root
	StartLine int
	EndLine   int
	NodeType  string // e.g. "function_declaration", "class_definition"
	Language  string // e.g. "go", "typescript", "python"
	Body      string // full source text
	EmbedText string // truncated for embedding (max 3000 chars)
}

// SearchResult is returned by similarity search.
type SearchResult struct {
	Name        string
	FilePath    string
	StartLine   int
	EndLine     int
	NodeType    string
	Language    string
	Distance    float32
	Similarity  float32
	BodyPreview string
}
