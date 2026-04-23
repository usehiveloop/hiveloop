package model_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// ---------------------------------------------------------------------------
// Pure-logic tests — no DB, no mocks (nothing to mock).
// ---------------------------------------------------------------------------

// TestIndexingStatus_IsTerminal pins the terminal-vs-running partition
// the scheduler uses to decide whether an attempt can be retried.
// If this partition drifts, the indexing queue either stalls (false
// positive — scheduler thinks in-flight work is done) or thrashes
// (false negative — scheduler relaunches a finished attempt). Both
// are user-visible outages.
