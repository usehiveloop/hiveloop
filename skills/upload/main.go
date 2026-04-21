// Command upload reads all skill.json manifests from the skills/ directory,
// fetches remote reference files, assembles bundles, and upserts them via the
// Hiveloop API. Existing skills (matched by name) are updated with a new
// inline version; missing skills are created.
//
// Usage:
//
//	go run ./skills/upload
//
// Requires HIVELOOP_SKILLS_API_KEY in the environment (or loaded via .env).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	apiBase    = "https://api.usehiveloop.com"
	batchSize  = 5
	maxRetries = 3
)

// ── Manifest types ──────────────────────────────────────────────────────────

type manifest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Root        string         `json:"root"`
	Files       []manifestFile `json:"files"`
}

type manifestFile struct {
	Path string `json:"path"`
	URL  string `json:"url"`
}

// ── API types ───────────────────────────────────────────────────────────────

type bundle struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Content     string      `json:"content"`
	References  []reference `json:"references"`
}

type reference struct {
	Path string `json:"path"`
	Body string `json:"body"`
}

type createRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SourceType  string  `json:"source_type"`
	Bundle      *bundle `json:"bundle,omitempty"`
}

type skillResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type listResponse struct {
	Data    []skillResponse `json:"data"`
	HasMore bool            `json:"has_more"`
}

// ── Skill loading ───────────────────────────────────────────────────────────

type loadedSkill struct {
	dir      string
	manifest manifest
	bundle   bundle
}

func discoverSkills(skillsDir string) ([]string, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("reading skills dir: %w", err)
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(skillsDir, entry.Name(), "skill.json")
		if _, err := os.Stat(manifestPath); err == nil {
			dirs = append(dirs, filepath.Join(skillsDir, entry.Name()))
		}
	}
	return dirs, nil
}

func loadSkill(dir string) (*loadedSkill, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "skill.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	var mf manifest
	if err := json.Unmarshal(manifestBytes, &mf); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Read root SKILL.md content.
	rootPath := mf.Root
	if rootPath == "" {
		rootPath = "./SKILL.md"
	}
	rootPath = filepath.Join(dir, strings.TrimPrefix(rootPath, "./"))
	rootContent, err := os.ReadFile(rootPath)
	if err != nil {
		return nil, fmt.Errorf("reading root %s: %w", rootPath, err)
	}

	// Fetch all reference files (local or remote).
	refs := make([]reference, 0, len(mf.Files))
	for _, file := range mf.Files {
		body, err := fetchFileContent(dir, file)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", file.Path, err)
		}
		refs = append(refs, reference{Path: file.Path, Body: body})
	}

	skillID := strings.ToLower(strings.ReplaceAll(mf.Name, " ", "-"))
	description := mf.Description

	return &loadedSkill{
		dir:      dir,
		manifest: mf,
		bundle: bundle{
			ID:          skillID,
			Title:       mf.Name,
			Description: description,
			Content:     string(rootContent),
			References:  refs,
		},
	}, nil
}

func fetchFileContent(dir string, file manifestFile) (string, error) {
	if file.URL != "" {
		return fetchURL(file.URL)
	}
	localPath := filepath.Join(dir, file.Path)
	content, err := os.ReadFile(localPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func fetchURL(url string) (string, error) {
	for attempt := range maxRetries {
		resp, err := http.Get(url)
		if err != nil {
			if attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return "", fmt.Errorf("GET %s: %w", url, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			if attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
		}
		if err != nil {
			return "", fmt.Errorf("reading body from %s: %w", url, err)
		}
		return string(body), nil
	}
	return "", fmt.Errorf("GET %s: exhausted retries", url)
}

// ── API client ──────────────────────────────────────────────────────────────

type apiClient struct {
	apiKey string
	http   *http.Client
}

func (client *apiClient) do(method, path string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, apiBase+path, reader)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+client.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.http.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, response.StatusCode, err
	}
	return responseBody, response.StatusCode, nil
}

func (client *apiClient) listAllSkills() (map[string]skillResponse, error) {
	skills := make(map[string]skillResponse)
	cursor := ""

	for {
		path := "/v1/skills?scope=own&limit=100"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		responseBody, status, err := client.do("GET", path, nil)
		if err != nil {
			return nil, fmt.Errorf("listing skills: %w", err)
		}
		if status != 200 {
			return nil, fmt.Errorf("listing skills: status %d: %s", status, string(responseBody))
		}
		var page listResponse
		if err := json.Unmarshal(responseBody, &page); err != nil {
			return nil, fmt.Errorf("parsing skills list: %w", err)
		}
		for _, skill := range page.Data {
			skills[skill.Name] = skill
		}
		if !page.HasMore {
			break
		}
		// Extract cursor from response.
		var raw map[string]any
		json.Unmarshal(responseBody, &raw)
		if nextCursor, ok := raw["next_cursor"].(string); ok {
			cursor = nextCursor
		} else {
			break
		}
	}
	return skills, nil
}

func (client *apiClient) createSkill(skill *loadedSkill) error {
	description := skill.manifest.Description
	request := createRequest{
		Name:        skill.manifest.Name,
		Description: &description,
		SourceType:  "inline",
		Bundle:      &skill.bundle,
	}
	responseBody, status, err := client.do("POST", "/v1/skills", request)
	if err != nil {
		return fmt.Errorf("creating skill %q: %w", skill.manifest.Name, err)
	}
	if status != 201 {
		return fmt.Errorf("creating skill %q: status %d: %s", skill.manifest.Name, status, string(responseBody))
	}
	return nil
}

func (client *apiClient) updateContent(skillID string, content *bundle) error {
	request := map[string]any{"bundle": content}
	responseBody, status, err := client.do("PUT", "/v1/skills/"+skillID+"/content", request)
	if err != nil {
		return fmt.Errorf("updating content for %s: %w", skillID, err)
	}
	if status != 200 {
		return fmt.Errorf("updating content for %s: status %d: %s", skillID, status, string(responseBody))
	}
	return nil
}

func (client *apiClient) updateMetadata(skillID string, name string, description string) error {
	request := map[string]any{"name": name, "description": description}
	responseBody, status, err := client.do("PATCH", "/v1/skills/"+skillID, request)
	if err != nil {
		return fmt.Errorf("updating metadata for %s: %w", skillID, err)
	}
	if status != 200 {
		return fmt.Errorf("updating metadata for %s: status %d: %s", skillID, status, string(responseBody))
	}
	return nil
}

// ── Main ────────────────────────────────────────────────────────────────────

func main() {
	apiKey := os.Getenv("HIVELOOP_SKILLS_API_KEY")
	if apiKey == "" {
		log.Fatal("HIVELOOP_SKILLS_API_KEY is required")
	}

	// Resolve skills/ directory relative to this script's location.
	scriptDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("getting working directory: %v", err)
	}
	skillsDir := filepath.Join(scriptDir, "skills")
	if _, err := os.Stat(skillsDir); err != nil {
		// Try from repo root.
		skillsDir = "skills"
	}

	log.Printf("Discovering skills in %s...", skillsDir)
	dirs, err := discoverSkills(skillsDir)
	if err != nil {
		log.Fatalf("discovering skills: %v", err)
	}
	if len(dirs) == 0 {
		log.Fatal("no skill.json manifests found")
	}
	log.Printf("Found %d skill(s)", len(dirs))

	// Load all skills and fetch remote references.
	log.Println("Loading skills and fetching remote references...")
	loaded := make([]*loadedSkill, 0, len(dirs))
	for _, dir := range dirs {
		skill, err := loadSkill(dir)
		if err != nil {
			log.Fatalf("loading %s: %v", dir, err)
		}
		refCount := len(skill.bundle.References)
		log.Printf("  %-30s  %d reference(s), %d bytes content",
			skill.manifest.Name, refCount, len(skill.bundle.Content))
		loaded = append(loaded, skill)
	}

	client := &apiClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 30 * time.Second},
	}

	// Fetch existing skills to determine create vs update.
	log.Println("Fetching existing skills from API...")
	existing, err := client.listAllSkills()
	if err != nil {
		log.Fatalf("listing existing skills: %v", err)
	}
	log.Printf("Found %d existing skill(s) on account", len(existing))

	// Process in batches.
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, batchSize)
	errors := make([]error, len(loaded))

	for index, skill := range loaded {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(index int, skill *loadedSkill) {
			defer wg.Done()
			defer func() { <-semaphore }()

			name := skill.manifest.Name
			if existingSkill, found := existing[name]; found {
				log.Printf("[%s] exists (id=%s), updating...", name, existingSkill.ID)
				if err := client.updateMetadata(existingSkill.ID, name, skill.manifest.Description); err != nil {
					errors[index] = err
					return
				}
				if err := client.updateContent(existingSkill.ID, &skill.bundle); err != nil {
					errors[index] = err
					return
				}
			} else {
				log.Printf("[%s] new, creating...", name)
				if err := client.createSkill(skill); err != nil {
					errors[index] = err
					return
				}
			}
			log.Printf("[%s] done", name)
		}(index, skill)
	}
	wg.Wait()

	// Report results.
	failed := 0
	for _, err := range errors {
		if err != nil {
			log.Printf("ERROR: %v", err)
			failed++
		}
	}
	if failed > 0 {
		log.Fatalf("%d/%d skill(s) failed", failed, len(loaded))
	}
	log.Printf("All %d skill(s) uploaded successfully", len(loaded))
}
