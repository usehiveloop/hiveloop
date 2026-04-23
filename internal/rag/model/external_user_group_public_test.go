package model_test

import ()

// setupExternalGroupSchema opens the test DB (which migrates the full
// RAG schema) plus per-test org / user / connection / source fixtures
// that every test in this file needs. The returned source ID is a FK
// target for the three external-group tables.
