package billing_test

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/billing/fake"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := billing.NewRegistry()
	p := fake.New("stripe")
	reg.Register(p)

	got, err := reg.Get("stripe")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got.Name() != "stripe" {
		t.Fatalf("Get: got %q, want %q", got.Name(), "stripe")
	}
}

func TestRegistry_GetUnknownReturnsError(t *testing.T) {
	reg := billing.NewRegistry()
	if _, err := reg.Get("nope"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRegistry_NamesSorted(t *testing.T) {
	reg := billing.NewRegistry()
	reg.Register(fake.New("stripe"))
	reg.Register(fake.New("paddle"))
	reg.Register(fake.New("polar"))

	names := reg.Names()
	if len(names) != 3 {
		t.Fatalf("Names: got %d, want 3", len(names))
	}
	// sorted alphabetically
	want := []string{"paddle", "polar", "stripe"}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("Names[%d]: got %q, want %q", i, names[i], n)
		}
	}
}
