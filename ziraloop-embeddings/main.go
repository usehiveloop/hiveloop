package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ziraloop/ziraloop-embeddings/internal/config"
	"github.com/ziraloop/ziraloop-embeddings/internal/drive"
	"github.com/ziraloop/ziraloop-embeddings/internal/drift"
	"github.com/ziraloop/ziraloop-embeddings/internal/embedder"
	"github.com/ziraloop/ziraloop-embeddings/internal/extractor"
	"github.com/ziraloop/ziraloop-embeddings/internal/store"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		runInit(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "similar":
		runSimilar(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "sql":
		runSQL(os.Args[2:])
	case "version":
		fmt.Printf("ziraloop-embeddings %s (%s)\n", version, commit)
	case "--help", "-h", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`ziraloop-embeddings — vector embedding index for codebases

Usage:
  ziraloop-embeddings init    --repos=path1,path2    Index repos, detect drift, upload to drive
  ziraloop-embeddings search  --query="..."           Semantic similarity search
  ziraloop-embeddings similar --file=... --function=  Find similar functions (no API call)
  ziraloop-embeddings status  [--repo=name]           Show index status
  ziraloop-embeddings sql     --query="SELECT ..."    Raw SQL query
  ziraloop-embeddings version                         Print version

Environment:
  ZIRALOOP_DRIVE_API_KEY        Agent-scoped drive token
  ZIRALOOP_DRIVE_ENDPOINT       Drive API base URL
  ZIRALOOP_EMBEDDING_ENDPOINT   Proxy URL for embedding calls
  ZIRALOOP_EMBEDDING_MODEL      Model name (default: text-embedding-3-large)
`)
}

func parseFlag(args []string, name string) string {
	prefix := "--" + name + "="
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return ""
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--"+name {
			return true
		}
	}
	return false
}

func runInit(args []string) {
	reposArg := parseFlag(args, "repos")
	if reposArg == "" {
		fmt.Fprintln(os.Stderr, "error: --repos is required")
		fmt.Fprintln(os.Stderr, "usage: ziraloop-embeddings init --repos=/path/to/repo1,/path/to/repo2")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	repoPaths := strings.Split(reposArg, ",")
	for idx := range repoPaths {
		repoPaths[idx] = strings.TrimSpace(repoPaths[idx])
	}

	totalStart := time.Now()

	// Check drive for existing vectors.db
	dbPath := cfg.DBPath
	driveClient := drive.NewClient(cfg.DriveEndpoint)

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("[drive] Checking for existing vectors.db...")
		if err := driveClient.PullIfExists(dbPath, "vectors.db"); err != nil {
			fmt.Printf("[drive] Warning: could not check drive: %v\n", err)
		}
	}

	// Open store
	db, err := store.Open(dbPath, cfg.EmbeddingDims)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	emb := embedder.New(cfg.EmbeddingEndpoint, cfg.EmbeddingModel, cfg.EmbeddingAPIKey)
	supportedExts := extractor.SupportedExtensions()

	for _, repoPath := range repoPaths {
		if err := indexRepo(db, emb, cfg, repoPath, supportedExts); err != nil {
			fmt.Fprintf(os.Stderr, "error indexing %s: %v\n", repoPath, err)
			os.Exit(1)
		}
	}

	// Upload to drive
	fmt.Println("[drive] Uploading vectors.db...")
	if err := driveClient.Upload(dbPath, "vectors.db"); err != nil {
		fmt.Printf("[drive] Warning: upload failed: %v\n", err)
	} else {
		fmt.Println("[drive] Upload complete.")
	}

	fmt.Printf("\nTotal time: %s\n", time.Since(totalStart).Round(time.Millisecond))
}

func indexRepo(db *store.Store, emb *embedder.Embedder, cfg *config.Config, repoPath string, supportedExts []string) error {
	repoName := extractor.RepoName(repoPath)
	fmt.Printf("\n=== Indexing %s ===\n", repoName)

	// Check drift
	meta, err := db.GetRepoMeta(repoName)
	if err != nil {
		return fmt.Errorf("reading repo meta: %w", err)
	}

	if meta != nil {
		driftResult := drift.Detect(repoPath, meta.LastCommit, supportedExts)

		switch driftResult.Status {
		case drift.NoChange:
			fmt.Printf("[drift] No changes since %s. Skipping.\n", meta.LastCommit[:8])
			return nil

		case drift.Changed:
			fmt.Printf("[drift] %d files changed since %s\n", len(driftResult.ChangedFiles), meta.LastCommit[:8])
			return incrementalIndex(db, emb, cfg, repoPath, repoName, driftResult)

		case drift.NoDrift:
			fmt.Printf("[drift] Warning: could not detect drift — %s. Re-indexing fully.\n", driftResult.Warning)

		case drift.Error:
			fmt.Printf("[drift] Warning: drift detection failed — %s. Re-indexing fully.\n", driftResult.Warning)
		}
	}

	return fullIndex(db, emb, cfg, repoPath, repoName)
}

func fullIndex(db *store.Store, emb *embedder.Embedder, cfg *config.Config, repoPath, repoName string) error {
	// Extract
	extractStart := time.Now()
	symbols, filesByLang, err := extractor.ScanAndExtract(repoPath)
	if err != nil {
		return fmt.Errorf("extraction: %w", err)
	}
	fmt.Printf("[extract] %d symbols from %v in %s\n", len(symbols), filesByLang, time.Since(extractStart).Round(time.Millisecond))

	if len(symbols) == 0 {
		fmt.Println("[extract] No symbols found. Skipping.")
		return nil
	}

	// Embed
	texts := make([]string, len(symbols))
	for idx, sym := range symbols {
		texts[idx] = sym.EmbedText
	}

	embedStart := time.Now()
	embeddings, totalTokens, batchTimings, err := emb.EmbedBatch(texts)
	if err != nil {
		return fmt.Errorf("embedding: %w", err)
	}
	fmt.Printf("[embed] %d embeddings in %s (%d tokens, $%.4f)\n",
		len(embeddings), time.Since(embedStart).Round(time.Millisecond),
		totalTokens, float64(totalTokens)*0.00000013)
	for _, bt := range batchTimings {
		fmt.Printf("  batch %d: %d symbols, %d tokens, %dms\n", bt.Batch, bt.Symbols, bt.Tokens, bt.DurationMS)
	}

	// Store
	storeStart := time.Now()
	if err := db.DeleteRepo(repoName); err != nil {
		return fmt.Errorf("clearing old data: %w", err)
	}

	for idx := range symbols {
		symbols[idx].RepoName = repoName
	}

	if err := db.InsertSymbols(symbols, embeddings); err != nil {
		return fmt.Errorf("inserting symbols: %w", err)
	}

	gitHead := drift.CurrentHead(repoPath)
	if err := db.SetRepoMeta(store.RepoMeta{
		RepoName:    repoName,
		RepoPath:    repoPath,
		LastCommit:  gitHead,
		Model:       cfg.EmbeddingModel,
		Dimensions:  cfg.EmbeddingDims,
		SymbolCount: len(symbols),
		TotalTokens: totalTokens,
		IndexedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("setting repo meta: %w", err)
	}
	fmt.Printf("[store] %d rows in %s\n", len(symbols), time.Since(storeStart).Round(time.Millisecond))

	return nil
}

func incrementalIndex(db *store.Store, emb *embedder.Embedder, cfg *config.Config, repoPath, repoName string, driftResult drift.Result) error {
	// Delete old symbols for changed files
	relPaths := make([]string, len(driftResult.ChangedFiles))
	for idx, filePath := range driftResult.ChangedFiles {
		relPaths[idx] = filePath
	}

	if err := db.DeleteSymbolsByFiles(repoName, relPaths); err != nil {
		return fmt.Errorf("deleting changed symbols: %w", err)
	}

	// Extract from changed files only
	extractStart := time.Now()
	symbols, err := extractor.ExtractFiles(repoPath, driftResult.ChangedFiles)
	if err != nil {
		return fmt.Errorf("extraction: %w", err)
	}
	fmt.Printf("[extract] %d symbols from %d changed files in %s\n",
		len(symbols), len(driftResult.ChangedFiles), time.Since(extractStart).Round(time.Millisecond))

	if len(symbols) == 0 {
		// Files changed but no extractable symbols — just update meta
		gitHead := drift.CurrentHead(repoPath)
		meta, _ := db.GetRepoMeta(repoName)
		if meta != nil {
			meta.LastCommit = gitHead
			meta.IndexedAt = time.Now().UTC().Format(time.RFC3339)
			return db.SetRepoMeta(*meta)
		}
		return nil
	}

	// Embed
	texts := make([]string, len(symbols))
	for idx, sym := range symbols {
		texts[idx] = sym.EmbedText
	}

	embedStart := time.Now()
	embeddings, totalTokens, _, err := emb.EmbedBatch(texts)
	if err != nil {
		return fmt.Errorf("embedding: %w", err)
	}
	fmt.Printf("[embed] %d embeddings in %s (%d tokens)\n",
		len(embeddings), time.Since(embedStart).Round(time.Millisecond), totalTokens)

	// Insert
	for idx := range symbols {
		symbols[idx].RepoName = repoName
	}
	if err := db.InsertSymbols(symbols, embeddings); err != nil {
		return fmt.Errorf("inserting symbols: %w", err)
	}

	// Update meta
	gitHead := drift.CurrentHead(repoPath)
	count, _ := db.SymbolCount(repoName)
	meta, _ := db.GetRepoMeta(repoName)
	if meta != nil {
		meta.LastCommit = gitHead
		meta.SymbolCount = count
		meta.TotalTokens += totalTokens
		meta.IndexedAt = time.Now().UTC().Format(time.RFC3339)
		return db.SetRepoMeta(*meta)
	}
	return nil
}

func runSearch(args []string) {
	query := parseFlag(args, "query")
	if query == "" {
		fmt.Fprintln(os.Stderr, "error: --query is required")
		os.Exit(1)
	}
	repoFilter := parseFlag(args, "repo")
	langFilter := parseFlag(args, "language")
	limitStr := parseFlag(args, "limit")
	limit := 10
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg.DBPath, cfg.EmbeddingDims)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	emb := embedder.New(cfg.EmbeddingEndpoint, cfg.EmbeddingModel, cfg.EmbeddingAPIKey)

	embedStart := time.Now()
	queryVec, err := emb.EmbedOne(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error embedding query: %v\n", err)
		os.Exit(1)
	}
	embedDuration := time.Since(embedStart)

	searchStart := time.Now()
	results, err := db.SearchSimilar(queryVec, limit, repoFilter, langFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error searching: %v\n", err)
		os.Exit(1)
	}
	searchDuration := time.Since(searchStart)

	fmt.Printf("Query: %q (embed: %s, search: %s)\n\n", query, embedDuration.Round(time.Millisecond), searchDuration.Round(time.Millisecond))
	for idx, result := range results {
		fmt.Printf("[%d] %s (%s) — similarity: %.4f\n", idx+1, result.Name, result.Language, result.Similarity)
		fmt.Printf("    %s:%d-%d\n", result.FilePath, result.StartLine, result.EndLine)
		preview := result.BodyPreview
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		fmt.Printf("    %s\n\n", preview)
	}
}

func runSimilar(args []string) {
	filePath := parseFlag(args, "file")
	funcName := parseFlag(args, "function")
	if filePath == "" || funcName == "" {
		fmt.Fprintln(os.Stderr, "error: --file and --function are required")
		os.Exit(1)
	}
	limitStr := parseFlag(args, "limit")
	limit := 10
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg.DBPath, cfg.EmbeddingDims)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Look up the function's stored vector — no API call needed
	searchStart := time.Now()
	vec, symbolID, err := db.GetSymbolVector(funcName, filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: function %q not found in %s\n", funcName, filePath)
		os.Exit(1)
	}

	results, err := db.SearchSimilarExcluding(vec, limit, symbolID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error searching: %v\n", err)
		os.Exit(1)
	}
	searchDuration := time.Since(searchStart)

	fmt.Printf("Similar to %s in %s (search: %s, no API call)\n\n", funcName, filePath, searchDuration.Round(time.Millisecond))
	for idx, result := range results {
		fmt.Printf("[%d] %s (%s) — similarity: %.4f\n", idx+1, result.Name, result.Language, result.Similarity)
		fmt.Printf("    %s:%d-%d\n", result.FilePath, result.StartLine, result.EndLine)
		preview := result.BodyPreview
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		fmt.Printf("    %s\n\n", preview)
	}
}

func runStatus(args []string) {
	repoFilter := parseFlag(args, "repo")

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg.DBPath, cfg.EmbeddingDims)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	stats, err := db.Stats(repoFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Database: %s (%.1f MB)\n\n", cfg.DBPath, stats.DBSizeMB)
	for _, repo := range stats.Repos {
		fmt.Printf("  %s\n", repo.RepoName)
		fmt.Printf("    Path:       %s\n", repo.RepoPath)
		fmt.Printf("    Commit:     %s\n", repo.LastCommit)
		fmt.Printf("    Symbols:    %d\n", repo.SymbolCount)
		fmt.Printf("    Model:      %s\n", repo.Model)
		fmt.Printf("    Indexed at: %s\n\n", repo.IndexedAt)
	}
	fmt.Printf("By language:\n")
	for _, langCount := range stats.ByLanguage {
		fmt.Printf("  %-15s %d\n", langCount.Language, langCount.Count)
	}
	fmt.Printf("\nBy type:\n")
	for _, typeCount := range stats.ByType {
		fmt.Printf("  %-30s %d\n", typeCount.NodeType, typeCount.Count)
	}
}

func runSQL(args []string) {
	query := parseFlag(args, "query")
	if query == "" {
		fmt.Fprintln(os.Stderr, "error: --query is required")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg.DBPath, cfg.EmbeddingDims)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	queryStart := time.Now()
	columns, rows, err := db.ExecSQL(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	queryDuration := time.Since(queryStart)

	// Print header
	for idx, col := range columns {
		if idx > 0 {
			fmt.Print("\t")
		}
		fmt.Print(col)
	}
	fmt.Println()

	// Print rows
	for _, row := range rows {
		for idx, col := range columns {
			if idx > 0 {
				fmt.Print("\t")
			}
			fmt.Print(row[col])
		}
		fmt.Println()
	}

	fmt.Printf("\n%d rows (%s)\n", len(rows), queryDuration.Round(time.Microsecond))
}
