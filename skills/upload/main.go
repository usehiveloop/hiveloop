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
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	apiBase    = "https://api.usehiveloop.com"
	batchSize  = 5
	maxRetries = 3
)

func main() {
	apiKey := os.Getenv("HIVELOOP_SKILLS_API_KEY")
	if apiKey == "" {
		log.Fatal("HIVELOOP_SKILLS_API_KEY is required")
	}

	scriptDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("getting working directory: %v", err)
	}
	skillsDir := filepath.Join(scriptDir, "skills")
	if _, err := os.Stat(skillsDir); err != nil {
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

	log.Println("Fetching existing skills from API...")
	existing, err := client.listAllSkills()
	if err != nil {
		log.Fatalf("listing existing skills: %v", err)
	}
	log.Printf("Found %d existing skill(s) on account", len(existing))

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
