package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// ============================================================
// Fake Slack API client (implements slackAPIClient)
// ============================================================

type fakeSlackAPI struct {
	mu          sync.Mutex
	channels    []SlackChannel
	users       map[string]*SlackUser
	history     map[string][]SlackMessage
	moreHistory map[string]bool
	replies     map[string][]SlackMessage
	members     map[string][]string
	calls       []string
}

func newFakeSlackAPI() *fakeSlackAPI {
	return &fakeSlackAPI{
		users:       make(map[string]*SlackUser),
		history:     make(map[string][]SlackMessage),
		moreHistory: make(map[string]bool),
		replies:     make(map[string][]SlackMessage),
		members:     make(map[string][]string),
	}
}

func (f *fakeSlackAPI) authTest(_ context.Context) (*authTestResponse, error) {
	return &authTestResponse{OK: true, URL: "https://test.slack.com/", Team: "Test", TeamID: "T001"}, nil
}

func (f *fakeSlackAPI) listChannels(_ context.Context, _ string) ([]SlackChannel, error) {
	f.mu.Lock()
	f.calls = append(f.calls, "conversations.list")
	f.mu.Unlock()
	return f.channels, nil
}

func (f *fakeSlackAPI) getChannelHistory(_ context.Context, channelID, oldest, latest string) ([]SlackMessage, bool, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("conversations.history ch=%s oldest=%s latest=%s", channelID, oldest, latest))
	msgs := f.history[channelID]
	hasMore := f.moreHistory[channelID]
	f.mu.Unlock()
	return msgs, hasMore, nil
}

func (f *fakeSlackAPI) getThreadReplies(_ context.Context, channelID, threadTS string) ([]SlackMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("conversations.replies ch=%s ts=%s", channelID, threadTS))
	key := channelID + "|" + threadTS
	msgs := f.replies[key]
	f.mu.Unlock()
	return msgs, nil
}

func (f *fakeSlackAPI) getUserInfo(_ context.Context, userID string) (*SlackUser, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("users.info u=%s", userID))
	u, ok := f.users[userID]
	f.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("user_not_found")
	}
	return u, nil
}

func (f *fakeSlackAPI) conversationMembers(_ context.Context, channelID string) ([]string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("conversations.members ch=%s", channelID))
	mems := f.members[channelID]
	f.mu.Unlock()
	return mems, nil
}

func (f *fakeSlackAPI) setChannels(chs ...SlackChannel) { f.channels = chs }
func (f *fakeSlackAPI) setUser(id, name, email string) {
	f.users[id] = &SlackUser{
		ID: id, Name: name, RealName: name,
		Profile: UserProfile{DisplayName: name, Email: email},
	}
}
func (f *fakeSlackAPI) setHistory(channelID string, msgs []SlackMessage, more bool) {
	f.history[channelID] = msgs
	f.moreHistory[channelID] = more
}
func (f *fakeSlackAPI) setReplies(channelID, threadTS string, msgs []SlackMessage) {
	f.replies[channelID+"|"+threadTS] = msgs
}
func (f *fakeSlackAPI) setMembers(channelID string, ids []string) { f.members[channelID] = ids }

type fixtureSource struct {
	cfg json.RawMessage
}

func (s *fixtureSource) SourceID() string        { return "src-fixture" }
func (s *fixtureSource) OrgID() string           { return "org-fixture" }
func (s *fixtureSource) SourceKind() string      { return Kind }
func (s *fixtureSource) Config() json.RawMessage { return s.cfg }

func makeChannel(id, name string, isMember, isPrivate bool) SlackChannel {
	return SlackChannel{
		ID: id, Name: name, NameNormalized: name,
		IsMember: isMember, IsPrivate: isPrivate,
		IsChannel: !isPrivate, IsGroup: isPrivate,
		IsArchived: false,
	}
}

func drainDocs(t *testing.T, ch <-chan interfaces.DocumentOrFailure) ([]*interfaces.Document, []*interfaces.ConnectorFailure) {
	t.Helper()
	var docs []*interfaces.Document
	var fails []*interfaces.ConnectorFailure
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return docs, fails
			}
			if ev.Doc != nil {
				docs = append(docs, ev.Doc)
			} else if ev.Failure != nil {
				fails = append(fails, ev.Failure)
			}
		case <-timeout:
			t.Fatal("drainDocs: timeout")
		}
	}
}

func drainAccesses(t *testing.T, ch <-chan interfaces.DocExternalAccessOrFailure) ([]*interfaces.DocExternalAccess, []*interfaces.ConnectorFailure) {
	t.Helper()
	var acc []*interfaces.DocExternalAccess
	var fails []*interfaces.ConnectorFailure
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return acc, fails
			}
			if ev.Access != nil {
				acc = append(acc, ev.Access)
			} else if ev.Failure != nil {
				fails = append(fails, ev.Failure)
			}
		case <-timeout:
			t.Fatal("drainAccesses: timeout")
		}
	}
}

func drainGroups(t *testing.T, ch <-chan interfaces.ExternalGroupOrFailure) ([]*interfaces.ExternalGroup, []*interfaces.ConnectorFailure) {
	t.Helper()
	var groups []*interfaces.ExternalGroup
	var fails []*interfaces.ConnectorFailure
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return groups, fails
			}
			if ev.Group != nil {
				groups = append(groups, ev.Group)
			} else if ev.Failure != nil {
				fails = append(fails, ev.Failure)
			}
		case <-timeout:
			t.Fatal("drainGroups: timeout")
		}
	}
}

func drainSlims(t *testing.T, ch <-chan interfaces.SlimDocOrFailure) ([]*interfaces.SlimDocument, []*interfaces.ConnectorFailure) {
	t.Helper()
	var slims []*interfaces.SlimDocument
	var fails []*interfaces.ConnectorFailure
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return slims, fails
			}
			if ev.Slim != nil {
				slims = append(slims, ev.Slim)
			} else if ev.Failure != nil {
				fails = append(fails, ev.Failure)
			}
		case <-timeout:
			t.Fatal("drainSlims: timeout")
		}
	}
}
