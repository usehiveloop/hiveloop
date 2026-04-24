package credentials_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakePicker is a test double for credentials.Picker.
type fakePicker struct {
	byGroup map[string]*model.Credential
	calls   []string
	err     error
}

func (p *fakePicker) Pick(_ context.Context, group string) (*model.Credential, error) {
	p.calls = append(p.calls, group)
	if p.err != nil {
		return nil, p.err
	}
	if cred, ok := p.byGroup[group]; ok {
		return cred, nil
	}
	return nil, credentials.ErrNoSystemCredential
}

func TestResolve_NilAgent(t *testing.T) {
	_, err := credentials.Resolve(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil agent")
	}
}

func TestResolve_PlatformAgentCallsPicker(t *testing.T) {
	sysCred := &model.Credential{
		ID:         uuid.New(),
		ProviderID: "moonshotai",
		IsSystem:   true,
	}
	picker := &fakePicker{byGroup: map[string]*model.Credential{"kimi": sysCred}}

	agent := &model.Agent{
		ID:            uuid.New(),
		CredentialID:  nil,
		ProviderGroup: "kimi",
	}

	// nil db is fine — BYOK branch is skipped, so no DB call happens.
	got, err := credentials.Resolve(context.Background(), nil, picker, agent)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != sysCred.ID {
		t.Errorf("got credential %s, want %s", got.ID, sysCred.ID)
	}
	if len(picker.calls) != 1 || picker.calls[0] != "kimi" {
		t.Errorf("picker calls = %v, want [kimi]", picker.calls)
	}
}

func TestResolve_PlatformAgentWithoutProviderGroupErrors(t *testing.T) {
	agent := &model.Agent{
		ID:            uuid.New(),
		CredentialID:  nil,
		ProviderGroup: "", // platform-keys agent must declare a provider
	}
	_, err := credentials.Resolve(context.Background(), nil, &fakePicker{}, agent)
	if err == nil {
		t.Fatal("expected error for platform agent without ProviderGroup")
	}
}

func TestResolve_PlatformAgentWithoutPickerErrors(t *testing.T) {
	agent := &model.Agent{
		ID:            uuid.New(),
		CredentialID:  nil,
		ProviderGroup: "kimi",
	}
	_, err := credentials.Resolve(context.Background(), nil, nil, agent)
	if err == nil {
		t.Fatal("expected error when picker is nil but agent needs one")
	}
}

func TestResolve_PickerErrorPropagates(t *testing.T) {
	sentinel := errors.New("picker boom")
	picker := &fakePicker{err: sentinel}
	agent := &model.Agent{
		ID:            uuid.New(),
		CredentialID:  nil,
		ProviderGroup: "kimi",
	}
	_, err := credentials.Resolve(context.Background(), nil, picker, agent)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error to propagate, got %v", err)
	}
}
