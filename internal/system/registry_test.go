package system

import "testing"

func validTask(name string) Task {
	return Task{
		Name:               name,
		Version:            "v1",
		ProviderGroup:      "openai",
		ModelTier:          ModelCheapest,
		UserPromptTemplate: "say hi",
		MaxOutputTokens:    16,
	}
}

func TestRegister_LookupRoundtrip(t *testing.T) {
	ResetForTest()
	Register(validTask("first"))
	got, ok := Lookup("first")
	if !ok {
		t.Fatalf("Lookup(%q) returned false", "first")
	}
	if got.Name != "first" {
		t.Fatalf("got %q, want %q", got.Name, "first")
	}
}

func TestLookup_Unknown(t *testing.T) {
	ResetForTest()
	if _, ok := Lookup("nope"); ok {
		t.Fatalf("Lookup of unregistered task returned ok=true")
	}
}

func TestRegister_DuplicatePanics(t *testing.T) {
	ResetForTest()
	Register(validTask("dup"))
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on duplicate Register, got none")
		}
	}()
	Register(validTask("dup"))
}

func TestRegister_RejectsMissingFields(t *testing.T) {
	cases := []struct {
		name string
		fix  func(*Task)
	}{
		{"missing-name", func(t *Task) { t.Name = "" }},
		{"missing-version", func(t *Task) { t.Version = "" }},
		{"missing-provider", func(t *Task) { t.ProviderGroup = "" }},
		{"missing-tier", func(t *Task) { t.ModelTier = "" }},
		{"named-without-model", func(t *Task) { t.ModelTier = ModelNamed; t.Model = "" }},
		{"missing-template", func(t *Task) { t.UserPromptTemplate = "" }},
		{"zero-max-tokens", func(t *Task) { t.MaxOutputTokens = 0 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ResetForTest()
			task := validTask("x")
			c.fix(&task)
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("expected panic, got none")
				}
			}()
			Register(task)
		})
	}
}

func TestRegister_DuplicateArgPanics(t *testing.T) {
	ResetForTest()
	task := validTask("dup-args")
	task.Args = []ArgSpec{
		{Name: "x", Type: ArgString},
		{Name: "x", Type: ArgString},
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic, got none")
		}
	}()
	Register(task)
}

func TestAll_SortedByName(t *testing.T) {
	ResetForTest()
	Register(validTask("zebra"))
	Register(validTask("alpha"))
	Register(validTask("mike"))
	got := All()
	names := []string{got[0].Name, got[1].Name, got[2].Name}
	want := []string{"alpha", "mike", "zebra"}
	for i := range names {
		if names[i] != want[i] {
			t.Fatalf("All()[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}
