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
	picker := &fakePicker{byModel: map[string]*model.Credential{"moonshotai/kimi-k2-instruct": sysCred}}

	agent := &model.Agent{
		ID:           uuid.New(),
		CredentialID: nil,
		Model:        "moonshotai/kimi-k2-instruct",
	}

	got, err := credentials.Resolve(context.Background(), nil, picker, agent)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != sysCred.ID {
		t.Errorf("got credential %s, want %s", got.ID, sysCred.ID)
	}
	if len(picker.modelCalls) != 1 || picker.modelCalls[0] != "moonshotai/kimi-k2-instruct" {
		t.Errorf("picker model calls = %v, want [moonshotai/kimi-k2-instruct]", picker.modelCalls)
	}
}

func TestResolve_PlatformAgentWithoutModelErrors(t *testing.T) {
	agent := &model.Agent{
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
	agent := &model.Agent{
		ID:           uuid.New(),
		CredentialID: nil,
		Model:        "moonshotai/kimi-k2-instruct",
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
		ID:           uuid.New(),
		CredentialID: nil,
		Model:        "moonshotai/kimi-k2-instruct",
	}
	_, err := credentials.Resolve(context.Background(), nil, picker, agent)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error to propagate, got %v", err)
	}
}
