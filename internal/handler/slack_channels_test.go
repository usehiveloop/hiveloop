package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/slackapp"
)

func TestSlackChannelHandler_AvailableChannels_PublicAndJoinedPrivate(t *testing.T) {
	h := NewSlackChannelHandler(nil, nil)
	h.listPublicChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{
			{ID: "C1", Name: "general", IsMember: false},
			{ID: "C2", Name: "product", IsMember: true},
			{ID: "C3", Name: "archived", IsArchived: true},
		}, nil
	}
	h.listBotChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{
			{ID: "C2", Name: "product", IsMember: true},
			{ID: "G1", Name: "leadership", IsPrivate: true, IsMember: true},
			{ID: "G2", Name: "private-not-member", IsPrivate: true, IsMember: false},
		}, nil
	}

	channels, err := h.availableChannels(context.Background(), "xoxb-test")
	if err != nil {
		t.Fatalf("availableChannels: %v", err)
	}
	if len(channels) != 3 {
		t.Fatalf("len = %d, want 3: %#v", len(channels), channels)
	}
	byID := map[string]slackapp.Channel{}
	for _, ch := range channels {
		byID[ch.ID] = ch
	}
	if byID["C1"].ID == "" || byID["C1"].IsPrivate {
		t.Fatalf("missing public C1: %#v", byID["C1"])
	}
	if !byID["C2"].IsMember {
		t.Fatalf("public joined channel should be marked member: %#v", byID["C2"])
	}
	if !byID["G1"].IsPrivate || !byID["G1"].IsMember {
		t.Fatalf("joined private channel missing: %#v", byID["G1"])
	}
	if byID["G2"].ID != "" || byID["C3"].ID != "" {
		t.Fatalf("unexpected archived/not-member private channels: %#v", byID)
	}
}

func TestSlackChannelHandler_JoinRequestedChannels(t *testing.T) {
	h := NewSlackChannelHandler(nil, nil)
	h.listPublicChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{
			{ID: "C1", Name: "general"},
			{ID: "C2", Name: "product", IsMember: true},
		}, nil
	}
	h.listBotChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{{ID: "G1", Name: "private", IsPrivate: true, IsMember: true}}, nil
	}
	h.joinChannel = func(_ context.Context, _ string, channelID string) (slackapp.Channel, error) {
		if channelID == "C1" {
			return slackapp.Channel{ID: channelID, Name: "general", IsMember: true}, nil
		}
		return slackapp.Channel{}, errors.New("unexpected join")
	}

	result, err := h.joinRequestedChannels(context.Background(), "xoxb-test", joinSlackChannelsRequest{
		ChannelIDs: []string{"C1", "C2", "G1", "G-missing"},
	})
	if err != nil {
		t.Fatalf("joinRequestedChannels: %v", err)
	}
	if result.Joined != 1 || result.AlreadyMember != 2 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	if !result.publicReady {
		t.Fatal("publicReady = false, want true after joining public channel")
	}
	if result.allReady {
		t.Fatal("allReady = true, want false when one requested channel failed")
	}
	if len(result.Failures) != 1 || result.Failures[0].ChannelID != "G-missing" {
		t.Fatalf("failures = %#v", result.Failures)
	}
}

func TestSlackChannelHandler_JoinRequestedChannels_AllSelectedReady(t *testing.T) {
	h := NewSlackChannelHandler(nil, nil)
	h.listPublicChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{
			{ID: "C1", Name: "general"},
			{ID: "C2", Name: "product", IsMember: true},
		}, nil
	}
	h.listBotChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return nil, nil
	}
	h.joinChannel = func(_ context.Context, _ string, channelID string) (slackapp.Channel, error) {
		if channelID != "C1" {
			return slackapp.Channel{}, errors.New("unexpected join")
		}
		return slackapp.Channel{ID: channelID, Name: "general", IsMember: true}, nil
	}

	result, err := h.joinRequestedChannels(context.Background(), "xoxb-test", joinSlackChannelsRequest{
		ChannelIDs: []string{"C1", "C2"},
	})
	if err != nil {
		t.Fatalf("joinRequestedChannels: %v", err)
	}
	if result.Joined != 1 || result.AlreadyMember != 1 || result.Failed != 0 {
		t.Fatalf("result = %#v", result)
	}
	if !result.publicReady {
		t.Fatal("publicReady = false, want true after selected public channel is available")
	}
	if !result.allReady {
		t.Fatal("allReady = false, want true when every selected channel is available")
	}
}

func TestSlackChannelHandler_JoinAllPublic(t *testing.T) {
	h := NewSlackChannelHandler(nil, nil)
	h.listPublicChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{
			{ID: "C1", Name: "general"},
			{ID: "C2", Name: "product"},
		}, nil
	}
	h.listBotChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{{ID: "G1", Name: "private", IsPrivate: true, IsMember: true}}, nil
	}
	h.joinChannel = func(_ context.Context, _ string, channelID string) (slackapp.Channel, error) {
		return slackapp.Channel{ID: channelID, IsMember: true}, nil
	}

	result, err := h.joinRequestedChannels(context.Background(), "xoxb-test", joinSlackChannelsRequest{AllPublic: true})
	if err != nil {
		t.Fatalf("joinRequestedChannels: %v", err)
	}
	if result.Joined != 2 || result.AlreadyMember != 0 || result.Failed != 0 {
		t.Fatalf("result = %#v", result)
	}
	if !result.publicReady {
		t.Fatal("publicReady = false, want true after joining public channels")
	}
	if !result.allReady {
		t.Fatal("allReady = false, want true after joining every public channel")
	}
}

func TestSlackChannelHandler_JoinedPrivateDoesNotCompleteOnboarding(t *testing.T) {
	h := NewSlackChannelHandler(nil, nil)
	h.listPublicChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{{ID: "C1", Name: "general"}}, nil
	}
	h.listBotChannels = func(context.Context, string) ([]slackapp.Channel, error) {
		return []slackapp.Channel{{ID: "G1", Name: "private", IsPrivate: true, IsMember: true}}, nil
	}
	h.joinChannel = func(context.Context, string, string) (slackapp.Channel, error) {
		return slackapp.Channel{}, errors.New("private-only test should not join")
	}

	result, err := h.joinRequestedChannels(context.Background(), "xoxb-test", joinSlackChannelsRequest{
		ChannelIDs: []string{"G1"},
	})
	if err != nil {
		t.Fatalf("joinRequestedChannels: %v", err)
	}
	if result.Joined != 0 || result.AlreadyMember != 1 || result.Failed != 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.publicReady {
		t.Fatal("publicReady = true, want false for private-only availability")
	}
}
