package fakebridge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	bridgepkg "github.com/usehivy/hivy/internal/bridge"
)

func (s *Server) registerRoutes() {
	s.Handler.HandleFunc("/push/agents/", s.handleUpsertAgent)
	s.Handler.HandleFunc("/agents/", s.handleAgentsRouter)
	s.Handler.HandleFunc("/conversations/", s.handleConversationsRouter)
	s.Handler.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","uptime_secs":1}`))
	})
}

func (s *Server) handleUpsertAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	agentID := strings.TrimPrefix(r.URL.Path, "/push/agents/")
	if agentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.EnforceOneAgent {
		for existing := range s.loaded {
			if existing != agentID {
				s.mu.Unlock()
				http.Error(w, `{"error":"only one agent per instance"}`, http.StatusConflict)
				return
			}
		}
	}

	var def bridgepkg.AgentDefinition
	if err := json.Unmarshal(body, &def); err == nil {
		s.captured.UpsertAgents = append(s.captured.UpsertAgents, def)
	}
	s.captured.UpsertAgentsRaw = append(s.captured.UpsertAgentsRaw, append([]byte(nil), body...))
	s.loaded[agentID] = true
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleAgentsRouter(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/agents/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	agentID := parts[0]

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		// HasAgent: always 404 so the pusher always pushes.
		w.WriteHeader(http.StatusNotFound)
		return

	case len(parts) == 2 && parts[1] == "conversations" && r.Method == http.MethodPost:
		s.handleCreateConversation(w, r, agentID)
		return

	case len(parts) == 4 && parts[1] == "conversations" && parts[3] == "approvals" && r.Method == http.MethodGet:
		s.handleListApprovals(w, r, agentID, parts[2])
		return

	case len(parts) == 5 && parts[1] == "conversations" && parts[3] == "approvals" && r.Method == http.MethodPost:
		s.handleResolveApproval(w, r, agentID, parts[2], parts[4])
		return

	case len(parts) == 4 && parts[1] == "conversations" && parts[3] == "approvals" && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"resolved":[],"not_found":[]}`))
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request, agentID string) {
	body, _ := io.ReadAll(r.Body)
	convID := fmt.Sprintf("conv-%d", time.Now().UnixNano())
	s.mu.Lock()
	s.captured.CreateConversations = append(s.captured.CreateConversations, CreatedConversation{
		AgentID:        agentID,
		ConversationID: convID,
		Body:           append(json.RawMessage(nil), body...),
	})
	s.mu.Unlock()

	resp := bridgepkg.CreateConversationResponse{
		ConversationId: convID,
		StreamUrl:      s.URL + "/conversations/" + convID + "/stream",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request, agentID, convID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if s.captured.PendingApprovals == nil {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	_ = json.NewEncoder(w).Encode(s.captured.PendingApprovals)
}

func (s *Server) handleResolveApproval(w http.ResponseWriter, r *http.Request, agentID, convID, requestID string) {
	body, _ := io.ReadAll(r.Body)
	var reply struct {
		Decision string `json:"decision"`
	}
	_ = json.Unmarshal(body, &reply)

	s.mu.Lock()
	s.captured.Approvals = append(s.captured.Approvals, ApprovalCall{
		AgentID:        agentID,
		ConversationID: convID,
		RequestID:      requestID,
		Body:           append(json.RawMessage(nil), body...),
		Decision:       reply.Decision,
	})
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"request_id":%q,"status":"resolved"}`, requestID)))
}

func (s *Server) handleConversationsRouter(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/conversations/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	convID := parts[0]

	switch {
	case len(parts) == 1 && r.Method == http.MethodDelete:
		s.mu.Lock()
		s.captured.Cancels = append(s.captured.Cancels, CancelCall{ConversationID: convID, Kind: "end"})
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ended"}`))
		return

	case len(parts) == 2 && parts[1] == "abort" && r.Method == http.MethodPost:
		s.mu.Lock()
		s.captured.Cancels = append(s.captured.Cancels, CancelCall{ConversationID: convID, Kind: "abort"})
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		return

	case len(parts) == 2 && parts[1] == "messages" && r.Method == http.MethodPost:
		s.handleSendMessage(w, r, convID)
		return

	case len(parts) == 2 && parts[1] == "stream" && r.Method == http.MethodGet:
		s.handleSSEStream(w, r, convID)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request, convID string) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(body, &req)

	s.mu.Lock()
	s.captured.Messages = append(s.captured.Messages, SentMessage{
		ConversationID: convID,
		Body:           append(json.RawMessage(nil), body...),
		Content:        req.Content,
	})
	s.mu.Unlock()

	if s.PushOnMessage && s.ScriptedSSE != nil {
		agentID := ""
		s.mu.Lock()
		for _, cc := range s.captured.CreateConversations {
			if cc.ConversationID == convID {
				agentID = cc.AgentID
				break
			}
		}
		s.mu.Unlock()
		events := s.ScriptedSSE(agentID, convID)
		s.QueueEvents(convID, events)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

func (s *Server) handleSSEStream(w http.ResponseWriter, r *http.Request, convID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	events := s.drainQueue(convID)

	if len(events) == 0 && s.ScriptedSSE != nil {
		agentID := ""
		s.mu.Lock()
		for _, cc := range s.captured.CreateConversations {
			if cc.ConversationID == convID {
				agentID = cc.AgentID
				break
			}
		}
		s.mu.Unlock()
		events = s.ScriptedSSE(agentID, convID)
	}

	for _, ev := range events {
		dataBytes, _ := json.Marshal(ev.Data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.EventType, string(dataBytes))
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Drain any events queued by a concurrent message turn before closing.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		more := s.drainQueue(convID)
		for _, ev := range more {
			dataBytes, _ := json.Marshal(ev.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.EventType, string(dataBytes))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if len(more) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
}
