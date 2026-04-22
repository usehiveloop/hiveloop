package embedder

import "gorm.io/gorm"

// SeedFromEntriesForTest exposes the injectable seeder core to the
// external _test package. Available only under the `_test.go` build
// constraint, so production code cannot reach the mutating seam.
//
// Used by TestSeedRegistry_UpdatesOnRegistryChange to verify that a
// modified registry entry propagates to the DB on re-seed, without
// touching the global Registry() function (which would risk leaking
// state across tests).
func SeedFromEntriesForTest(db *gorm.DB, entries []RegistryEntry) error {
	return seedFromEntries(db, entries)
}

// DeriveDatasetNameForTest exposes the unexported deriveDatasetName
// helper for pure table-driven testing.
func DeriveDatasetNameForTest(provider, modelName string, dim int) string {
	return deriveDatasetName(provider, modelName, dim)
}
