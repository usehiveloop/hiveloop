package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// The standard GraphQL introspection query.
const introspectionQuery = `{
  __schema {
    queryType { name }
    mutationType { name }
    types {
      kind
      name
      description
      fields(includeDeprecated: false) {
        name
        description
        args {
          name
          description
          type {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
                ofType {
                  kind
                  name
                }
              }
            }
          }
          defaultValue
        }
        type {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
      inputFields {
        name
        description
        type {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}`

// IntrospectionResponse represents the response from a GraphQL introspection query.
type IntrospectionResponse struct {
	Data struct {
		Schema IntrospectionSchema `json:"__schema"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// IntrospectionSchema represents the __schema field.
type IntrospectionSchema struct {
	QueryType    *TypeRef          `json:"queryType"`
	MutationType *TypeRef          `json:"mutationType"`
	Types        []IntrospectType  `json:"types"`
}

// TypeRef is a named type reference.
type TypeRef struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	OfType *TypeRef `json:"ofType"`
}

// IntrospectType represents a type in the schema.
type IntrospectType struct {
	Kind        string             `json:"kind"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Fields      []IntrospectField  `json:"fields"`
	InputFields []IntrospectInput  `json:"inputFields"`
}

// IntrospectField represents a field on a type.
type IntrospectField struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Args        []IntrospectInput `json:"args"`
	Type        TypeRef           `json:"type"`
}

// IntrospectInput represents an input field or argument.
type IntrospectInput struct {
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Type         TypeRef `json:"type"`
	DefaultValue *string `json:"defaultValue"`
}

// runIntrospection sends an introspection query and returns the parsed schema.
func runIntrospection(url string) (*IntrospectionSchema, error) {
	body, err := json.Marshal(map[string]string{
		"query": introspectionQuery,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspection request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspection returned status %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 300)]))
	}

	var result IntrospectionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing introspection response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("introspection errors: %s", result.Errors[0].Message)
	}

	return &result.Data.Schema, nil
}

// getNamedType resolves a potentially wrapped type (NON_NULL, LIST) to its named type.
func getNamedType(t TypeRef) string {
	if t.Name != "" {
		return t.Name
	}
	if t.OfType != nil {
		return getNamedType(*t.OfType)
	}
	return ""
}

// isNonNull checks if a type is NON_NULL (i.e., required).
func isNonNull(t TypeRef) bool {
	return t.Kind == "NON_NULL"
}

// graphqlTypeToJSONSchema maps GraphQL scalar types to JSON Schema types.
func graphqlTypeToJSONSchema(t TypeRef) string {
	named := getNamedType(t)
	switch named {
	case "String", "ID", "DateTime", "Date", "URI", "URL":
		return "string"
	case "Int":
		return "integer"
	case "Float":
		return "number"
	case "Boolean":
		return "boolean"
	default:
		// Input objects, enums, etc.
		return "string"
	}
}
