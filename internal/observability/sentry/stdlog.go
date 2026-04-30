package sentry

import (
	stdlog "log"
	"log/slog"
)

func NewStdlogBridge(component string) *stdlog.Logger {
	return stdlog.New(slogWriter{logger: slog.Default(), component: component}, "", 0)
}

type slogWriter struct {
	logger    *slog.Logger
	component string
}

func (sw slogWriter) Write(payload []byte) (int, error) {
	message := string(payload)
	if length := len(message); length > 0 && message[length-1] == '\n' {
		message = message[:length-1]
	}
	sw.logger.Error(sw.component+" stdlog", "message", message)
	return len(payload), nil
}
