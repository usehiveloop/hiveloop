package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type paginationCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type paginatedResponse[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more"`
}

func parsePagination(r *http.Request) (int, *paginationCursor, error) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			return 0, nil, fmt.Errorf("invalid limit")
		}
		if n > 100 {
			n = 100
		}
		limit = n
	}

	var cursor *paginationCursor
	if c := r.URL.Query().Get("cursor"); c != "" && c != "0" {
		var err error
		cursor, err = decodeCursor(c)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid cursor")
		}
	}

	return limit, cursor, nil
}

func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	s := createdAt.Format(time.RFC3339Nano) + "|" + id.String()
	return base64.URLEncoding.EncodeToString([]byte(s))
}

func decodeCursor(s string) (*paginationCursor, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	parts := splitOnce(string(b), '|')
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}
	return &paginationCursor{CreatedAt: t, ID: id}, nil
}

func splitOnce(s string, sep byte) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func applyPagination(q *gorm.DB, cursor *paginationCursor, limit int) *gorm.DB {
	if cursor != nil {
		q = q.Where("(created_at, id) < (?, ?)", cursor.CreatedAt, cursor.ID)
	}
	return q.Order("created_at DESC, id DESC").Limit(limit + 1)
}
