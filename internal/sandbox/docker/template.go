package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/errdefs"

	"github.com/usehivy/hivy/internal/sandbox"
)

const defaultTemplateBaseImage = "node:22-bookworm-slim"

func (d *Driver) BuildTemplate(ctx context.Context, opts sandbox.TemplateBuildRequest) (string, error) {
	return d.BuildTemplateWithLogs(ctx, opts, nil)
}

func (d *Driver) BuildTemplateWithLogs(ctx context.Context, opts sandbox.TemplateBuildRequest, onLog func(string)) (string, error) {
	tag := "hivy-template:" + imageTagName(opts.Name)
	dockerfile := templateDockerfile(opts)
	buildCtx, err := dockerfileContext(dockerfile)
	if err != nil {
		return "", err
	}

	response, err := d.cli.ImageBuild(ctx, buildCtx, build.ImageBuildOptions{
		Tags:       []string{tag},
		Dockerfile: "Dockerfile",
		Remove:     true,
		Labels: d.labels(map[string]string{
			"template_name": opts.Name,
			"template":      "true",
		}),
		CPUQuota: buildCPUQuota(opts.CPU),
		Memory:   int64(opts.Memory) * bytesPerGiB,
	})
	if err != nil {
		return "", fmt.Errorf("building docker image %s: %w", tag, err)
	}
	defer response.Body.Close()

	if err := streamBuildLogs(response.Body, onLog); err != nil {
		return "", err
	}
	return tag, nil
}

func (d *Driver) GetTemplateStatus(ctx context.Context, externalID string) (*sandbox.TemplateBuildStatus, error) {
	if _, err := d.cli.ImageInspect(ctx, externalID); err != nil {
		if errdefs.IsNotFound(err) {
			return &sandbox.TemplateBuildStatus{
				State:    "error",
				ErrorMsg: "docker image not found",
			}, nil
		}
		return nil, fmt.Errorf("inspecting docker image %s: %w", externalID, err)
	}
	return &sandbox.TemplateBuildStatus{State: "ready"}, nil
}

func (d *Driver) GetTemplateLogs(context.Context, string) (string, error) {
	return "", nil
}

func (d *Driver) DeleteTemplate(ctx context.Context, externalID string) error {
	_, err := d.cli.ImageRemove(ctx, externalID, image.RemoveOptions{Force: true, PruneChildren: false})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("removing docker image %s: %w", externalID, err)
	}
	return nil
}

func templateDockerfile(opts sandbox.TemplateBuildRequest) string {
	baseImage := strings.TrimSpace(opts.BaseImage)
	if baseImage == "" {
		baseImage = defaultTemplateBaseImage
	}

	var b strings.Builder
	b.WriteString("FROM ")
	b.WriteString(baseImage)
	b.WriteByte('\n')
	for _, cmd := range opts.BuildCommands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		b.WriteString("RUN ")
		b.WriteString(cmd)
		b.WriteByte('\n')
	}
	return b.String()
}

func dockerfileContext(dockerfile string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	header := &tar.Header{
		Name:    "Dockerfile",
		Mode:    0o644,
		Size:    int64(len(dockerfile)),
		ModTime: time.Unix(0, 0),
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("writing Dockerfile header: %w", err)
	}
	if _, err := tw.Write([]byte(dockerfile)); err != nil {
		return nil, fmt.Errorf("writing Dockerfile: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing Dockerfile tar: %w", err)
	}
	return bytes.NewReader(buf.Bytes()), nil
}

func streamBuildLogs(r io.Reader, onLog func(string)) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		var event struct {
			Stream string `json:"stream"`
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			emitBuildLog(onLog, line)
			continue
		}
		if event.Error != "" {
			emitBuildLog(onLog, event.Error)
			return fmt.Errorf("docker image build failed: %s", strings.TrimSpace(event.Error))
		}
		emitBuildLog(onLog, strings.TrimSpace(event.Stream))
		emitBuildLog(onLog, strings.TrimSpace(event.Status))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading docker image build stream: %w", err)
	}
	return nil
}

func emitBuildLog(onLog func(string), line string) {
	line = strings.TrimSpace(line)
	if onLog != nil && line != "" {
		onLog(line)
	}
}

func buildCPUQuota(cpu int) int64 {
	if cpu <= 0 {
		return 0
	}
	return int64(cpu) * 100000
}
