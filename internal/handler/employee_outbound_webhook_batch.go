package handler

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (h *EmployeeOutboundWebhookHandler) HandleBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sb, ok := h.loadSandbox(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBatchBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	if !h.verifySignature(ctx, sb, body, r.Header.Get("X-Hiveloop-Signature")) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	reader := io.Reader(bytes.NewReader(body))
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid gzip batch"})
			return
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}

	count := 0
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event employeeOutboundEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid batch event"})
			return
		}
		if event.At.IsZero() {
			event.At = h.now().UTC()
		}
		h.storeAndMaybeEnqueue(ctx, sb, &event)
		count++
	}
	if err := scanner.Err(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read batch"})
		return
	}
	if err := h.db.WithContext(ctx).Model(sb).Update("last_active_at", h.now()).Error; err != nil {
		captureEmployeeWebhookIngest(ctx, "update_last_active_batch", sb, nil, "", "", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "count": count})
}
