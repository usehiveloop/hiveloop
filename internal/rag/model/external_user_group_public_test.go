package model_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// setupExternalGroupSchema opens the test DB (which migrates the full
// RAG schema) plus per-test org / user / connection / source fixtures
// that every test in this file needs. The returned source ID is a FK
// target for the three external-group tables.
