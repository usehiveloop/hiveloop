package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type hub struct {
	mu       sync.Mutex
	clients  map[string]*websocket.Conn
	upgrader websocket.Upgrader
}

func newHub() *hub {
	return &hub{
		clients: map[string]*websocket.Conn{},
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

func (h *hub) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") == "" {
		http.NotFound(w, r)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "error", err)
		return
	}

	id := newID()
	h.mu.Lock()
	h.clients[id] = conn
	h.mu.Unlock()

	ack := wsAck{MessageType: "connection_ack", WSClientID: id}
	if err := conn.WriteJSON(ack); err != nil {
		h.unregister(id)
		return
	}

	go h.readLoop(id, conn)
}

func (h *hub) readLoop(id string, conn *websocket.Conn) {
	defer h.unregister(id)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *hub) unregister(id string) {
	h.mu.Lock()
	conn, ok := h.clients[id]
	delete(h.clients, id)
	h.mu.Unlock()
	if ok {
		_ = conn.Close()
	}
}

func (h *hub) sendSuccess(wsClientID, providerConfigKey, connectionID string) {
	h.send(wsClientID, wsSuccess{
		MessageType:       "success",
		ProviderConfigKey: providerConfigKey,
		ConnectionID:      connectionID,
	})
}

func (h *hub) sendError(wsClientID, providerConfigKey, connectionID, errType, errDesc string) {
	h.send(wsClientID, wsErr{
		MessageType:       "error",
		ProviderConfigKey: providerConfigKey,
		ConnectionID:      connectionID,
		ErrorType:         errType,
		ErrorDesc:         errDesc,
	})
}

func (h *hub) send(id string, payload any) {
	h.mu.Lock()
	conn, ok := h.clients[id]
	h.mu.Unlock()
	if !ok {
		slog.Warn("ws: no client for id", "ws_client_id", id)
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, body); err != nil {
		h.unregister(id)
	}
}
