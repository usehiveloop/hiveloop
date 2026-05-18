package handler_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEmployeeSQLiteBackup_ExtendsReadDeadlineForSlowBody(t *testing.T) {
	streamer := &capturingBackupStreamer{}
	h := newSQLiteBackupHarnessWithStreamer(t, 1024*1024, true, streamer, nil)
	server := httptest.NewUnstartedServer(h.router)
	server.Config.ReadTimeout = 20 * time.Millisecond
	server.Start()
	defer server.Close()

	body := []byte("slow-backup")
	req, err := http.NewRequest(
		http.MethodPut,
		server.URL+"/internal/employees/"+h.agentID.String()+"/sqlite-backup",
		&slowReadCloser{data: body, delay: 30 * time.Millisecond},
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("Authorization", "Bearer "+h.bridgeKey)
	req.ContentLength = int64(len(body))

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("upload backup: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		got, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(got))
	}
	if !bytes.Equal(streamer.body, body) {
		t.Fatalf("streamed body mismatch: %q", streamer.body)
	}
}

type capturingBackupStreamer struct {
	body []byte
}

func (s *capturingBackupStreamer) Stream(_ context.Context, _ string, body io.Reader, _ string) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.body = data
	return nil
}

type slowReadCloser struct {
	data  []byte
	delay time.Duration
}

func (r *slowReadCloser) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func (r *slowReadCloser) Close() error {
	return nil
}
