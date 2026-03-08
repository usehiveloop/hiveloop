package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

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
