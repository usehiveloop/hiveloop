package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

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
