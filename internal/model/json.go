package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// RawJSON stores arbitrary JSON values (objects, arrays, strings, etc.) in JSONB columns.
// Unlike JSON (which is map[string]any), RawJSON can represent any valid JSON value.
type RawJSON json.RawMessage

func (r RawJSON) Value() (driver.Value, error) {
	if len(r) == 0 {
		return "null", nil
	}
	return string(r), nil
}

func (r *RawJSON) Scan(value any) error {
	if value == nil {
		*r = RawJSON("null")
		return nil
	}
	switch v := value.(type) {
	case string:
		*r = RawJSON(v)
	case []byte:
		*r = RawJSON(v)
	default:
		return fmt.Errorf("unsupported type for RawJSON: %T", value)
	}
	return nil
}

func (r RawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return []byte(r), nil
}

func (r *RawJSON) UnmarshalJSON(data []byte) error {
	*r = RawJSON(data)
	return nil
}

// JSON is a custom type for JSONB columns in PostgreSQL.
type JSON map[string]any

func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, fmt.Errorf("marshaling JSON: %w", err)
	}
	return string(b), nil
}

func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = JSON{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("unsupported type for JSON: %T", value)
	}

	return json.Unmarshal(bytes, j)
}
