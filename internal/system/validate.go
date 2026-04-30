package system

import "fmt"

// ValidationError describes one failed argument check. Errors is a flat list
// — the handler returns the first to the caller as a 400.
type ValidationError struct {
	Arg     string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("arg %q: %s", e.Arg, e.Message)
}

// ValidateArgs checks args against the task's declared ArgSpecs.
//
// Behaviour:
//   - Required missing → error.
//   - Type mismatch (incl. JSON-decoded numbers vs ArgInt) → error.
//   - String exceeding MaxLen → error.
//   - String list elements exceeding MaxLen → error.
//   - Int outside [Min, Max] → error.
//   - Unknown args (present but not declared) → error. Strict on purpose: a
//     typo in the frontend shouldn't silently no-op, and tasks own their
//     schema.
func ValidateArgs(args map[string]any, specs []ArgSpec) []ValidationError {
	var errs []ValidationError
	declared := map[string]ArgSpec{}
	for _, s := range specs {
		declared[s.Name] = s
	}
	for k := range args {
		if _, ok := declared[k]; !ok {
			errs = append(errs, ValidationError{
				Arg:     k,
				Message: "unknown argument",
			})
		}
	}
	for _, s := range specs {
		val, present := args[s.Name]
		if !present {
			if s.Required {
				errs = append(errs, ValidationError{
					Arg:     s.Name,
					Message: "required",
				})
			}
			continue
		}
		if e := validateOne(s, val); e != nil {
			errs = append(errs, *e)
		}
	}
	return errs
}

func validateOne(s ArgSpec, val any) *ValidationError {
	switch s.Type {
	case ArgString:
		v, ok := val.(string)
		if !ok {
			return &ValidationError{Arg: s.Name, Message: "must be a string"}
		}
		if s.MaxLen > 0 && len(v) > s.MaxLen {
			return &ValidationError{
				Arg:     s.Name,
				Message: fmt.Sprintf("exceeds MaxLen=%d (got %d)", s.MaxLen, len(v)),
			}
		}
	case ArgInt:
		// JSON decode produces float64 for numbers. Accept whole-number floats.
		var iv int
		switch t := val.(type) {
		case float64:
			if t != float64(int(t)) {
				return &ValidationError{Arg: s.Name, Message: "must be an integer"}
			}
			iv = int(t)
		case int:
			iv = t
		default:
			return &ValidationError{Arg: s.Name, Message: "must be an integer"}
		}
		if s.Min != nil && iv < *s.Min {
			return &ValidationError{
				Arg:     s.Name,
				Message: fmt.Sprintf("must be >= %d", *s.Min),
			}
		}
		if s.Max != nil && iv > *s.Max {
			return &ValidationError{
				Arg:     s.Name,
				Message: fmt.Sprintf("must be <= %d", *s.Max),
			}
		}
	case ArgBool:
		if _, ok := val.(bool); !ok {
			return &ValidationError{Arg: s.Name, Message: "must be a boolean"}
		}
	case ArgStringList:
		raw, ok := val.([]any)
		if !ok {
			return &ValidationError{Arg: s.Name, Message: "must be an array of strings"}
		}
		for i, item := range raw {
			str, ok := item.(string)
			if !ok {
				return &ValidationError{
					Arg:     s.Name,
					Message: fmt.Sprintf("element %d is not a string", i),
				}
			}
			if s.MaxLen > 0 && len(str) > s.MaxLen {
				return &ValidationError{
					Arg:     s.Name,
					Message: fmt.Sprintf("element %d exceeds MaxLen=%d", i, s.MaxLen),
				}
			}
		}
	case ArgObject:
		if _, ok := val.(map[string]any); !ok {
			return &ValidationError{Arg: s.Name, Message: "must be an object"}
		}
	case ArgObjectList:
		raw, ok := val.([]any)
		if !ok {
			return &ValidationError{Arg: s.Name, Message: "must be an array of objects"}
		}
		for i, item := range raw {
			if _, ok := item.(map[string]any); !ok {
				return &ValidationError{
					Arg:     s.Name,
					Message: fmt.Sprintf("element %d is not an object", i),
				}
			}
		}
	default:
		return &ValidationError{
			Arg:     s.Name,
			Message: fmt.Sprintf("unknown ArgType %q (server-side bug)", s.Type),
		}
	}
	return nil
}

