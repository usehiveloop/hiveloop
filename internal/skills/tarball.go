package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxReferenceBytes caps the body size of any single file loaded out of a
// skill tarball. Binary assets or oversized docs are skipped.
const maxReferenceBytes = 256 * 1024 // 256 KiB

// parsedSkill is the intermediate result of walking a skill tarball.
type parsedSkill struct {
	Manifest   map[string]any
	SkillBody  string
	References []Reference
}

// parseSkillTarball walks a .tar.gz stream and extracts the SKILL.md body +
// every sibling file under subpath. GitHub tarballs wrap everything in a
// top-level directory (e.g. owner-repo-abcdef/); that prefix is stripped, then
// subpath (optional) is applied on top.
//
// Skipped silently: files over maxReferenceBytes, non-regular entries, and
// anything outside the requested subpath.
func parseSkillTarball(r io.Reader, subpath string) (*parsedSkill, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	normalizedSubpath := strings.Trim(path.Clean("/"+subpath), "/")

	result := &parsedSkill{}
	skillFound := false

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		relative := stripTopDir(header.Name)
		if relative == "" {
			continue
		}

		if normalizedSubpath != "" {
			prefix := normalizedSubpath + "/"
			if relative != normalizedSubpath && !strings.HasPrefix(relative, prefix) {
				continue
			}
			relative = strings.TrimPrefix(relative, prefix)
		}
		if relative == "" {
			continue
		}

		if header.Size > maxReferenceBytes {
			continue
		}

		body, err := io.ReadAll(io.LimitReader(tr, maxReferenceBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", header.Name, err)
		}
		if int64(len(body)) > maxReferenceBytes {
			continue
		}

		if relative == "SKILL.md" {
			manifest, skillBody, err := parseFrontmatter(body)
			if err != nil {
				return nil, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
			}
			result.Manifest = manifest
			result.SkillBody = skillBody
			skillFound = true
			continue
		}

		result.References = append(result.References, Reference{
			Path: relative,
			Body: string(body),
		})
	}

	if !skillFound {
		return nil, fmt.Errorf("SKILL.md not found at subpath %q", subpath)
	}
	return result, nil
}

// stripTopDir removes the first path segment of a GitHub tarball entry
// (e.g. "hiveloop-skills-abc123/SKILL.md" -> "SKILL.md").
func stripTopDir(name string) string {
	name = strings.TrimPrefix(name, "./")
	idx := strings.Index(name, "/")
	if idx < 0 {
		return ""
	}
	return name[idx+1:]
}

// parseFrontmatter splits a SKILL.md body into YAML frontmatter + markdown.
// If there is no frontmatter, manifest is nil and the whole body is returned.
func parseFrontmatter(body []byte) (map[string]any, string, error) {
	if !bytes.HasPrefix(body, []byte("---")) {
		return nil, string(body), nil
	}

	// Find the closing delimiter. It must sit on its own line.
	after := body[3:]
	// Tolerate CRLF after the opening delimiter.
	after = bytes.TrimLeft(after, "\r\n")

	end := bytes.Index(after, []byte("\n---"))
	if end < 0 {
		return nil, string(body), nil
	}

	yamlBytes := after[:end]
	rest := after[end+4:] // skip "\n---"
	rest = bytes.TrimLeft(rest, "\r\n")

	var manifest map[string]any
	if err := yaml.Unmarshal(yamlBytes, &manifest); err != nil {
		return nil, "", fmt.Errorf("unmarshal frontmatter yaml: %w", err)
	}

	return manifest, string(rest), nil
}
