package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

	rootPath := mf.Root
	if rootPath == "" {
		rootPath = "./SKILL.md"
	}
	rootPath = filepath.Join(dir, strings.TrimPrefix(rootPath, "./"))
	rootContent, err := os.ReadFile(rootPath)
	if err != nil {
		return nil, fmt.Errorf("reading root %s: %w", rootPath, err)
	}

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
