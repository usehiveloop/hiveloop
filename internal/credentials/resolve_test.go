package credentials_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type fakePicker struct {
	byModel    map[string]*model.Credential
	modelCalls []string
	err        error
}

func (p *fakePicker) Pick(_ context.Context, _ string) (*model.Credential, error) {
	return nil, credentials.ErrNoSystemCredential
}

func (p *fakePicker) PickByModel(_ context.Context, modelID string) (*model.Credential, error) {
	p.modelCalls = append(p.modelCalls, modelID)
	if p.err != nil {
		return nil, p.err
	}
	if cred, ok := p.byModel[modelID]; ok {
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

func TestResolve_PlatformAgentCallsPickerByModel(t *testing.T) {
	sysCred := &model.Credential{
		ID:         uuid.New(),
		ProviderID: "moonshotai",
		IsSystem:   true,
	}
	picker := &fakePicker{byModel: map[string]*model.Credential{"kimi-k2.5": sysCred}}

	agent := &model.Employee{
		ID:           uuid.New(),
		CredentialID: nil,
		Model:        "kimi-k2.5",
	}

	got, err := credentials.Resolve(context.Background(), nil, picker, agent)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != sysCred.ID {
		t.Errorf("got credential %s, want %s", got.ID, sysCred.ID)
	}
	if len(picker.modelCalls) != 1 || picker.modelCalls[0] != "kimi-k2.5" {
		t.Errorf("picker model calls = %v, want [kimi-k2.5]", picker.modelCalls)
	}
}

func TestResolve_PlatformAgentWithoutModelErrors(t *testing.T) {
	agent := &model.Employee{
		ID:           uuid.New(),
		CredentialID: nil,
		Model:        "",
	}
	_, err := credentials.Resolve(context.Background(), nil, &fakePicker{}, agent)
	if err == nil {
		t.Fatal("expected error for platform agent without Model")
	}
}

func TestResolve_PlatformAgentWithoutPickerErrors(t *testing.T) {
	agent := &model.Employee{
		ID:           uuid.New(),
		CredentialID: nil,
		Model:        "kimi-k2.5",
	}
	_, err := credentials.Resolve(context.Background(), nil, nil, agent)
	if err == nil {
		t.Fatal("expected error when picker is nil but agent needs one")
	}
}

func TestResolve_PickerErrorPropagates(t *testing.T) {
	sentinel := errors.New("picker boom")
	picker := &fakePicker{err: sentinel}
	agent := &model.Employee{
		ID:           uuid.New(),
		CredentialID: nil,
		Model:        "kimi-k2.5",
	}
	_, err := credentials.Resolve(context.Background(), nil, picker, agent)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error to propagate, got %v", err)
	}
}
