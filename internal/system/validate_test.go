package system

import "testing"

func TestValidateArgs_RequiredMissing(t *testing.T) {
	specs := []ArgSpec{{Name: "shape", Type: ArgString, Required: true}}
	errs := ValidateArgs(map[string]any{}, specs)
	if len(errs) != 1 || errs[0].Arg != "shape" {
		t.Fatalf("expected one error on 'shape', got %+v", errs)
	}
}

func TestValidateArgs_OptionalMissingIsOK(t *testing.T) {
	specs := []ArgSpec{{Name: "topic", Type: ArgString, Required: false}}
	errs := ValidateArgs(map[string]any{}, specs)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %+v", errs)
	}
}

func TestValidateArgs_StringTypeMismatch(t *testing.T) {
	specs := []ArgSpec{{Name: "shape", Type: ArgString, Required: true}}
	errs := ValidateArgs(map[string]any{"shape": 42}, specs)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %+v", errs)
	}
}

func TestValidateArgs_StringMaxLen(t *testing.T) {
	specs := []ArgSpec{{Name: "s", Type: ArgString, MaxLen: 5}}
	errs := ValidateArgs(map[string]any{"s": "hello!"}, specs)
	if len(errs) != 1 {
		t.Fatalf("expected MaxLen violation, got %+v", errs)
	}
	errs = ValidateArgs(map[string]any{"s": "hello"}, specs)
	if len(errs) != 0 {
		t.Fatalf("at-MaxLen should pass, got %+v", errs)
	}
}

func TestValidateArgs_IntFromJSONFloat(t *testing.T) {
	min, max := 1, 10
	specs := []ArgSpec{{Name: "n", Type: ArgInt, Min: &min, Max: &max}}
	// JSON-decoded numbers are float64; whole numbers should be accepted.
	errs := ValidateArgs(map[string]any{"n": float64(5)}, specs)
	if len(errs) != 0 {
		t.Fatalf("whole-number float should pass, got %+v", errs)
	}
	errs = ValidateArgs(map[string]any{"n": float64(5.5)}, specs)
	if len(errs) != 1 {
		t.Fatalf("non-integer float should fail, got %+v", errs)
	}
	errs = ValidateArgs(map[string]any{"n": float64(0)}, specs)
	if len(errs) != 1 {
		t.Fatalf("below Min should fail, got %+v", errs)
	}
	errs = ValidateArgs(map[string]any{"n": float64(11)}, specs)
	if len(errs) != 1 {
		t.Fatalf("above Max should fail, got %+v", errs)
	}
}

func TestValidateArgs_BoolType(t *testing.T) {
	specs := []ArgSpec{{Name: "verbose", Type: ArgBool}}
	if errs := ValidateArgs(map[string]any{"verbose": true}, specs); len(errs) != 0 {
		t.Fatalf("bool true should pass, got %+v", errs)
	}
	if errs := ValidateArgs(map[string]any{"verbose": "yes"}, specs); len(errs) != 1 {
		t.Fatalf("string instead of bool should fail, got %+v", errs)
	}
}

func TestValidateArgs_StringList(t *testing.T) {
	specs := []ArgSpec{{Name: "tags", Type: ArgStringList, MaxLen: 4}}
	if errs := ValidateArgs(map[string]any{"tags": []any{"go", "ai"}}, specs); len(errs) != 0 {
		t.Fatalf("[]any of strings should pass, got %+v", errs)
	}
	if errs := ValidateArgs(map[string]any{"tags": []any{"go", 42}}, specs); len(errs) != 1 {
		t.Fatalf("non-string element should fail, got %+v", errs)
	}
	if errs := ValidateArgs(map[string]any{"tags": []any{"longer-than-four"}}, specs); len(errs) != 1 {
		t.Fatalf("element MaxLen violation should fail, got %+v", errs)
	}
	if errs := ValidateArgs(map[string]any{"tags": "not-a-list"}, specs); len(errs) != 1 {
		t.Fatalf("non-list should fail, got %+v", errs)
	}
}

func TestValidateArgs_UnknownArgRejected(t *testing.T) {
	specs := []ArgSpec{{Name: "shape", Type: ArgString, Required: true}}
	errs := ValidateArgs(map[string]any{"shape": "x", "extra": "y"}, specs)
	// Expect one error for the unknown 'extra'. Order depends on map
	// iteration so just check presence.
	hasExtra := false
	for _, e := range errs {
		if e.Arg == "extra" {
			hasExtra = true
		}
	}
	if !hasExtra {
		t.Fatalf("expected error on unknown arg 'extra', got %+v", errs)
	}
}
