package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// maxPeekBytes is the maximum number of bytes to read from the request body
// when extracting the model field. This avoids buffering large payloads.
const maxPeekBytes = 2048

// ExtractModel peeks at the request body to extract the "model" field from
// JSON payloads. It only attempts extraction for POST requests with JSON
// content types to paths that typically contain a model field.
// The request body is always left intact for the upstream.
func ExtractModel(req *http.Request) string {
	if req.Method != http.MethodPost || req.Body == nil {
		return ""
	}

	ct := req.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return ""
	}

	// Read up to maxPeekBytes from the body
	peek := make([]byte, maxPeekBytes)
	n, err := req.Body.Read(peek)
	if n == 0 {
		return ""
	}
	peek = peek[:n]

	// Reconstruct the body: peeked bytes + remaining
	if err == io.EOF {
		// Entire body fit in peek buffer
		req.Body = io.NopCloser(bytes.NewReader(peek))
	} else {
		req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), req.Body))
	}

	// Try fast JSON extraction of the "model" field
	return extractModelFromJSON(peek)
}

// extractModelFromJSON extracts the top-level "model" string field from
// a potentially incomplete JSON payload.
func extractModelFromJSON(data []byte) string {
	// Use json.Decoder for robustness with partial payloads
	dec := json.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return ""
	}
	// Must start with '{'
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return ""
	}

	// Scan top-level keys looking for "model"
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return ""
		}
		key, ok := t.(string)
		if !ok {
			return ""
		}
		if key == "model" {
			t, err = dec.Token()
			if err != nil {
				return ""
			}
			if s, ok := t.(string); ok {
				return s
			}
			return ""
		}
		// Skip value for non-model keys
		if !skipValue(dec) {
			return ""
		}
	}

	return ""
}

// skipValue skips a single JSON value in the decoder (object, array, or primitive).
func skipValue(dec *json.Decoder) bool {
	t, err := dec.Token()
	if err != nil {
		return false
	}
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			for dec.More() {
				// skip key
				if _, err := dec.Token(); err != nil {
					return false
				}
				// skip value
				if !skipValue(dec) {
					return false
				}
			}
			// consume closing '}'
			if _, err := dec.Token(); err != nil {
				return false
			}
		case '[':
			for dec.More() {
				if !skipValue(dec) {
					return false
				}
			}
			// consume closing ']'
			if _, err := dec.Token(); err != nil {
				return false
			}
		}
	}
	return true
}
