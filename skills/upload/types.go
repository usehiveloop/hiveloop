package main

import "encoding/json"

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

type loadedSkill struct {
	dir      string
	manifest manifest
	bundle   bundle
}
