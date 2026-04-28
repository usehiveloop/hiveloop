package main

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type integration struct {
	UniqueKey   string         `json:"unique_key"`
	Provider    string         `json:"provider"`
	DisplayName string         `json:"display_name"`
	Credentials map[string]any `json:"credentials,omitempty"`
	WebhookURL  string         `json:"webhook_url"`
	WebhookSecret string       `json:"webhook_secret"`
}

type connection struct {
	ID                string         `json:"connection_id"`
	ProviderConfigKey string         `json:"provider_config_key"`
	Provider          string         `json:"provider"`
	Credentials       map[string]any `json:"credentials"`
	ConnectionConfig  map[string]any `json:"connection_config"`
}

type connectSession struct {
	Token              string
	AllowedIntegrations []string
	EndUserID          string
	ExistingConnectionID string
	CreatedAt          time.Time
}

type oauthSession struct {
	State             string
	WSClientID        string
	ProviderConfigKey string
	Provider          string
	AuthMode          string
	ConnectionID      string
	CreatedAt         time.Time
}

type outcome struct {
	Result      string         // "approve" | "reject"
	ErrorType   string         // when reject
	ErrorDesc   string
	Credentials map[string]any // injected on approve
}

type callLog struct {
	When   time.Time `json:"when"`
	Method string    `json:"method"`
	Path   string    `json:"path"`
}

type proxyFixture struct {
	Method  string
	Path    string
	Pattern string
	Status  int
	Body    any
	Headers map[string]string
}

type store struct {
	mu sync.RWMutex

	integrations    map[string]*integration   // by uniqueKey
	connections     map[string]*connection    // by connectionID
	connectSessions map[string]*connectSession // by token
	oauthSessions   map[string]*oauthSession   // by state

	nextOutcome outcome // applied to next /oauth/connect or form-auth call
	calls       []callLog
	fixtures    []proxyFixture
}

func newStore() *store {
	return &store{
		integrations:    map[string]*integration{},
		connections:     map[string]*connection{},
		connectSessions: map[string]*connectSession{},
		oauthSessions:   map[string]*oauthSession{},
		nextOutcome:     outcome{Result: "approve"},
	}
}

func (s *store) putIntegration(i *integration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.integrations[i.UniqueKey] = i
}

func (s *store) getIntegration(key string) (*integration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.integrations[key]
	return i, ok
}

func (s *store) deleteIntegration(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.integrations, key)
}

func (s *store) putConnection(c *connection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[c.ID] = c
}

func (s *store) getConnection(id string) (*connection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.connections[id]
	return c, ok
}

func (s *store) deleteConnection(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.connections, id)
}

func (s *store) putConnectSession(sess *connectSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectSessions[sess.Token] = sess
}

func (s *store) getConnectSession(token string) (*connectSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.connectSessions[token]
	return sess, ok
}

func (s *store) putOAuthSession(sess *oauthSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oauthSessions[sess.State] = sess
}

func (s *store) takeOAuthSession(state string) (*oauthSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.oauthSessions[state]
	if ok {
		delete(s.oauthSessions, state)
	}
	return sess, ok
}

func (s *store) consumeOutcome() outcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.nextOutcome
	s.nextOutcome = outcome{Result: "approve"}
	return o
}

func (s *store) setOutcome(o outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextOutcome = o
}

func (s *store) recordCall(method, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, callLog{When: time.Now(), Method: method, Path: path})
}

func (s *store) snapshotCalls() []callLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]callLog, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *store) setFixtures(f []proxyFixture) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fixtures = f
}

func (s *store) findFixture(method, path string) (*proxyFixture, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.fixtures {
		f := &s.fixtures[i]
		if f.Method != "" && f.Method != "*" && f.Method != method {
			continue
		}
		if f.Path != "" && f.Path == path {
			return f, true
		}
		if f.Pattern != "" && globMatch(f.Pattern, path) {
			return f, true
		}
	}
	return nil, false
}

func globMatch(pattern, path string) bool {
	pp := splitPath(pattern)
	pa := splitPath(path)
	if len(pp) != len(pa) {
		return false
	}
	for i := range pp {
		if pp[i] == "*" {
			continue
		}
		if pp[i] != pa[i] {
			return false
		}
	}
	return true
}

func splitPath(p string) []string {
	out := []string{}
	cur := ""
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(p[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections = map[string]*connection{}
	s.connectSessions = map[string]*connectSession{}
	s.oauthSessions = map[string]*oauthSession{}
	s.nextOutcome = outcome{Result: "approve"}
	s.calls = nil
	s.fixtures = nil
}

func newID() string { return uuid.NewString() }
