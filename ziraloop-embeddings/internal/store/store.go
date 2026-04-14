package store

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"github.com/ziraloop/ziraloop-embeddings/internal/model"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	sqlite_vec.Auto()
}

type RepoMeta struct {
	RepoName    string
	RepoPath    string
	LastCommit  string
	Model       string
	Dimensions  int
	SymbolCount int
	TotalTokens int
	IndexedAt   string
}

type LangCount struct {
	Language string
	Count    int
}

type TypeCount struct {
	NodeType string
	Count    int
}

type Stats struct {
	DBSizeMB   float64
	Repos      []RepoMeta
	ByLanguage []LangCount
	ByType     []TypeCount
}

type Store struct {
	db   *sql.DB
	dims int
}

func Open(dbPath string, dims int) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Create schema
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	vecSQL := fmt.Sprintf(vecSchemaSQL, dims)
	if _, err := db.Exec(vecSQL); err != nil {
		return nil, fmt.Errorf("create vec table: %w", err)
	}

	return &Store{db: db, dims: dims}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func serializeF32(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for idx, val := range vec {
		binary.LittleEndian.PutUint32(buf[idx*4:], math.Float32bits(val))
	}
	return buf
}

// InsertSymbols inserts symbols and their embeddings in a transaction.
func (s *Store) InsertSymbols(symbols []model.Symbol, embeddings [][]float32) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	symStmt, err := tx.Prepare("INSERT INTO symbols (repo_name, name, file_path, start_line, end_line, node_type, language, body) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer symStmt.Close()

	vecStmt, err := tx.Prepare("INSERT INTO vec_symbols (rowid, embedding) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer vecStmt.Close()

	for idx, sym := range symbols {
		result, err := symStmt.Exec(sym.RepoName, sym.Name, sym.FilePath, sym.StartLine, sym.EndLine, sym.NodeType, sym.Language, sym.Body)
		if err != nil {
			return fmt.Errorf("insert symbol %s: %w", sym.Name, err)
		}
		rowID, _ := result.LastInsertId()

		if idx < len(embeddings) && embeddings[idx] != nil {
			if _, err := vecStmt.Exec(rowID, serializeF32(embeddings[idx])); err != nil {
				return fmt.Errorf("insert vector for %s: %w", sym.Name, err)
			}
		}
	}

	return tx.Commit()
}

// DeleteRepo removes all symbols and vectors for a repo.
func (s *Store) DeleteRepo(repoName string) error {
	rows, err := s.db.Query("SELECT id FROM symbols WHERE repo_name = ?", repoName)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var symbolID int64
		rows.Scan(&symbolID)
		ids = append(ids, symbolID)
	}

	if len(ids) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, symbolID := range ids {
		tx.Exec("DELETE FROM vec_symbols WHERE rowid = ?", symbolID)
	}
	tx.Exec("DELETE FROM symbols WHERE repo_name = ?", repoName)
	tx.Exec("DELETE FROM repo_meta WHERE repo_name = ?", repoName)

	return tx.Commit()
}

// DeleteSymbolsByFiles removes symbols for specific files in a repo.
func (s *Store) DeleteSymbolsByFiles(repoName string, filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, filePath := range filePaths {
		rows, err := tx.Query("SELECT id FROM symbols WHERE repo_name = ? AND file_path = ?", repoName, filePath)
		if err != nil {
			continue
		}
		var ids []int64
		for rows.Next() {
			var symbolID int64
			rows.Scan(&symbolID)
			ids = append(ids, symbolID)
		}
		rows.Close()

		for _, symbolID := range ids {
			tx.Exec("DELETE FROM vec_symbols WHERE rowid = ?", symbolID)
		}
		tx.Exec("DELETE FROM symbols WHERE repo_name = ? AND file_path = ?", repoName, filePath)
	}

	return tx.Commit()
}

// GetRepoMeta returns metadata for a repo, or nil if not indexed.
func (s *Store) GetRepoMeta(repoName string) (*RepoMeta, error) {
	row := s.db.QueryRow("SELECT repo_name, repo_path, last_commit, model, dimensions, symbol_count, total_tokens, indexed_at FROM repo_meta WHERE repo_name = ?", repoName)
	var meta RepoMeta
	err := row.Scan(&meta.RepoName, &meta.RepoPath, &meta.LastCommit, &meta.Model, &meta.Dimensions, &meta.SymbolCount, &meta.TotalTokens, &meta.IndexedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

// SetRepoMeta upserts repo metadata.
func (s *Store) SetRepoMeta(meta RepoMeta) error {
	_, err := s.db.Exec(`
		INSERT INTO repo_meta (repo_name, repo_path, last_commit, model, dimensions, symbol_count, total_tokens, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_name) DO UPDATE SET
			repo_path = excluded.repo_path,
			last_commit = excluded.last_commit,
			model = excluded.model,
			dimensions = excluded.dimensions,
			symbol_count = excluded.symbol_count,
			total_tokens = excluded.total_tokens,
			indexed_at = excluded.indexed_at
	`, meta.RepoName, meta.RepoPath, meta.LastCommit, meta.Model, meta.Dimensions, meta.SymbolCount, meta.TotalTokens, meta.IndexedAt)
	return err
}

// SymbolCount returns the number of symbols for a repo.
func (s *Store) SymbolCount(repoName string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM symbols WHERE repo_name = ?", repoName).Scan(&count)
	return count, err
}

// SearchSimilar finds the most similar symbols to a query vector.
func (s *Store) SearchSimilar(queryVec []float32, limit int, repoFilter, langFilter string) ([]model.SearchResult, error) {
	vecBytes := serializeF32(queryVec)
	fetchLimit := limit * 5 // over-fetch for filtering

	rows, err := s.db.Query(`
		SELECT vec_symbols.rowid, vec_symbols.distance
		FROM vec_symbols
		WHERE embedding MATCH ?
		ORDER BY distance
		LIMIT ?
	`, vecBytes, fetchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.SearchResult
	for rows.Next() {
		var rowID int64
		var distance float32
		if err := rows.Scan(&rowID, &distance); err != nil {
			continue
		}

		sym := s.db.QueryRow("SELECT name, file_path, start_line, end_line, node_type, language, body FROM symbols WHERE id = ?", rowID)
		var name, filePath, nodeType, language, body string
		var startLine, endLine int
		if err := sym.Scan(&name, &filePath, &startLine, &endLine, &nodeType, &language, &body); err != nil {
			continue
		}

		if repoFilter != "" {
			var symRepo string
			s.db.QueryRow("SELECT repo_name FROM symbols WHERE id = ?", rowID).Scan(&symRepo)
			if symRepo != repoFilter {
				continue
			}
		}
		if langFilter != "" && language != langFilter {
			continue
		}

		preview := body
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}

		results = append(results, model.SearchResult{
			Name:        name,
			FilePath:    filePath,
			StartLine:   startLine,
			EndLine:     endLine,
			NodeType:    nodeType,
			Language:    language,
			Distance:    distance,
			Similarity:  1 - distance,
			BodyPreview: preview,
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GetSymbolVector returns the stored embedding vector for a symbol.
func (s *Store) GetSymbolVector(funcName, filePath string) ([]float32, int64, error) {
	var symbolID int64
	err := s.db.QueryRow("SELECT id FROM symbols WHERE name = ? AND file_path = ? LIMIT 1", funcName, filePath).Scan(&symbolID)
	if err != nil {
		return nil, 0, fmt.Errorf("symbol not found: %w", err)
	}

	var vecBytes []byte
	err = s.db.QueryRow("SELECT embedding FROM vec_symbols WHERE rowid = ?", symbolID).Scan(&vecBytes)
	if err != nil {
		return nil, 0, fmt.Errorf("vector not found: %w", err)
	}

	vec := make([]float32, len(vecBytes)/4)
	for idx := range vec {
		bits := binary.LittleEndian.Uint32(vecBytes[idx*4:])
		vec[idx] = math.Float32frombits(bits)
	}

	return vec, symbolID, nil
}

// SearchSimilarExcluding searches for similar symbols, excluding one by ID.
func (s *Store) SearchSimilarExcluding(queryVec []float32, limit int, excludeID int64) ([]model.SearchResult, error) {
	results, err := s.SearchSimilar(queryVec, limit+1, "", "")
	if err != nil {
		return nil, err
	}

	// Filter out the excluded symbol
	filtered := make([]model.SearchResult, 0, limit)
	for _, result := range results {
		if len(filtered) >= limit {
			break
		}
		// Skip the source symbol by matching name+file
		var symbolID int64
		s.db.QueryRow("SELECT id FROM symbols WHERE name = ? AND file_path = ? LIMIT 1", result.Name, result.FilePath).Scan(&symbolID)
		if symbolID != excludeID {
			filtered = append(filtered, result)
		}
	}
	return filtered, nil
}

// ExecSQL runs a raw SQL query and returns columns and rows.
func (s *Store) ExecSQL(query string) ([]string, []map[string]interface{}, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for idx := range values {
			valuePtrs[idx] = &values[idx]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{}, len(columns))
		for idx, col := range columns {
			row[col] = values[idx]
		}
		results = append(results, row)
	}

	return columns, results, nil
}

// Stats returns database statistics.
func (s *Store) Stats(repoFilter string) (*Stats, error) {
	stats := &Stats{}

	// DB size
	dbPath := ""
	s.db.QueryRow("PRAGMA database_list").Scan(nil, nil, &dbPath)
	if info, err := os.Stat(dbPath); err == nil {
		stats.DBSizeMB = float64(info.Size()) / (1024 * 1024)
	}

	// Repos
	repoQuery := "SELECT repo_name, repo_path, last_commit, model, dimensions, symbol_count, total_tokens, indexed_at FROM repo_meta"
	repoArgs := []interface{}{}
	if repoFilter != "" {
		repoQuery += " WHERE repo_name = ?"
		repoArgs = append(repoArgs, repoFilter)
	}
	repoRows, err := s.db.Query(repoQuery, repoArgs...)
	if err != nil {
		return nil, err
	}
	defer repoRows.Close()

	for repoRows.Next() {
		var meta RepoMeta
		repoRows.Scan(&meta.RepoName, &meta.RepoPath, &meta.LastCommit, &meta.Model, &meta.Dimensions, &meta.SymbolCount, &meta.TotalTokens, &meta.IndexedAt)
		stats.Repos = append(stats.Repos, meta)
	}

	// By language
	langRows, _ := s.db.Query("SELECT language, COUNT(*) as count FROM symbols GROUP BY language ORDER BY count DESC")
	if langRows != nil {
		defer langRows.Close()
		for langRows.Next() {
			var lc LangCount
			langRows.Scan(&lc.Language, &lc.Count)
			stats.ByLanguage = append(stats.ByLanguage, lc)
		}
	}

	// By type
	typeRows, _ := s.db.Query("SELECT node_type, COUNT(*) as count FROM symbols GROUP BY node_type ORDER BY count DESC")
	if typeRows != nil {
		defer typeRows.Close()
		for typeRows.Next() {
			var tc TypeCount
			typeRows.Scan(&tc.NodeType, &tc.Count)
			stats.ByType = append(stats.ByType, tc)
		}
	}

	return stats, nil
}
