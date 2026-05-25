package docker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/errdefs"
	"github.com/usehivy/hivy/internal/sandbox"
)

const integrationLabelPrefix = "hivytest"

func TestMain(m *testing.M) {
	code := runIntegrationTests(m)
	os.Exit(code)
}

func runIntegrationTests(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	driver, err := NewDriver(Config{
		PublicHost:           "127.0.0.1",
		ContainerLabelPrefix: integrationLabelPrefix,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewDriver: %v\n", err)
		return 1
	}
	if err := driver.Validate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Validate: %v\n", err)
		return 1
	}
	if err := cleanupIntegrationArtifacts(ctx, driver); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup integration artifacts: %v\n", err)
		return 1
	}
	return m.Run()
}

func TestDockerDriverLifecycleEndpointExecAndResources(t *testing.T) {
	ctx := context.Background()
	driver := newIntegrationDriver(t, ctx)
	imageRef := buildIntegrationImage(t, ctx, driver, "runtime", integrationRuntimeDockerfile())

	info, err := driver.CreateSandbox(ctx, sandbox.CreateSandboxOpts{
		Name:        "hivy-docker-integration",
		TemplateRef: imageRef,
		EnvVars:     map[string]string{"HIVY_DOCKER_TEST": "ok"},
		Labels:      map[string]string{"test": "docker-driver"},
		CPU:         1,
		Memory:      1,
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}

	url, err := driver.GetEndpoint(ctx, info.ExternalID, 8080)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	assertHTTPBody(t, ctx, url, "ok")

	output, err := driver.ExecuteCommand(ctx, info.ExternalID, "printf '%s' \"$HIVY_DOCKER_TEST\"")
	if err != nil {
		t.Fatalf("ExecuteCommand env: %v output=%q", err, output)
	}
	if output != "ok" {
		t.Fatalf("env output = %q, want ok", output)
	}

	if output, err = driver.ExecuteCommand(ctx, info.ExternalID, "echo persisted > /tmp/hivy-state"); err != nil {
		t.Fatalf("ExecuteCommand write state: %v output=%q", err, output)
	}
	if err := driver.StopSandbox(ctx, info.ExternalID); err != nil {
		t.Fatalf("StopSandbox: %v", err)
	}
	status, err := driver.GetStatus(ctx, info.ExternalID)
	if err != nil {
		t.Fatalf("GetStatus stopped: %v", err)
	}
	if status != sandbox.StatusStopped {
		t.Fatalf("status after stop = %s, want %s", status, sandbox.StatusStopped)
	}
	if err := driver.StartSandbox(ctx, info.ExternalID); err != nil {
		t.Fatalf("StartSandbox: %v", err)
	}
	output, err = driver.ExecuteCommand(ctx, info.ExternalID, "cat /tmp/hivy-state")
	if err != nil {
		t.Fatalf("ExecuteCommand read state: %v output=%q", err, output)
	}
	if strings.TrimSpace(output) != "persisted" {
		t.Fatalf("persisted state = %q, want persisted", output)
	}

	usage, err := driver.GetResourceUsage(ctx, info.ExternalID)
	if err != nil {
		t.Fatalf("GetResourceUsage: %v", err)
	}
	if usage.MemoryLimitBytes != bytesPerGiB {
		t.Fatalf("memory limit = %d, want %d", usage.MemoryLimitBytes, bytesPerGiB)
	}
	if usage.CPUQuota == "" {
		t.Fatalf("CPUQuota should be populated for a CPU-limited container")
	}
}

func TestDockerDriverBuildTemplateCreatesLocalImage(t *testing.T) {
	ctx := context.Background()
	driver := newIntegrationDriver(t, ctx)
	baseRef := buildIntegrationImage(t, ctx, driver, "template-base", integrationRuntimeDockerfile())

	templateRef, err := driver.BuildTemplateWithLogs(ctx, sandbox.TemplateBuildRequest{
		Name:          fmt.Sprintf("integration-%d", time.Now().UnixNano()),
		BaseImage:     baseRef,
		BuildCommands: []string{"echo built > /template-marker"},
	}, func(string) {})
	if err != nil {
		t.Fatalf("BuildTemplateWithLogs: %v", err)
	}

	status, err := driver.GetTemplateStatus(ctx, templateRef)
	if err != nil {
		t.Fatalf("GetTemplateStatus: %v", err)
	}
	if status.State != "ready" {
		t.Fatalf("template status = %q, want ready", status.State)
	}

	info, err := driver.CreateSandbox(ctx, sandbox.CreateSandboxOpts{
		Name:        "hivy-docker-template-integration",
		TemplateRef: templateRef,
	})
	if err != nil {
		t.Fatalf("CreateSandbox from template: %v", err)
	}

	output, err := driver.ExecuteCommand(ctx, info.ExternalID, "cat /template-marker")
	if err != nil {
		t.Fatalf("ExecuteCommand template marker: %v output=%q", err, output)
	}
	if strings.TrimSpace(output) != "built" {
		t.Fatalf("template marker = %q, want built", output)
	}
}

func newIntegrationDriver(t *testing.T, ctx context.Context) *Driver {
	t.Helper()

	driver, err := NewDriver(Config{
		PublicHost:           "127.0.0.1",
		ContainerLabelPrefix: integrationLabelPrefix,
	})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	if err := driver.Validate(ctx); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return driver
}

func buildIntegrationImage(t *testing.T, ctx context.Context, driver *Driver, name string, dockerfile string) string {
	t.Helper()

	ref := fmt.Sprintf("hivy-docker-test-%s:%d", name, time.Now().UnixNano())
	buildCtx, err := dockerfileContext(dockerfile)
	if err != nil {
		t.Fatalf("dockerfileContext: %v", err)
	}
	response, err := driver.cli.ImageBuild(ctx, buildCtx, build.ImageBuildOptions{
		Tags:       []string{ref},
		Dockerfile: "Dockerfile",
		Remove:     true,
	})
	if err != nil {
		t.Fatalf("ImageBuild %s: %v", ref, err)
	}
	defer response.Body.Close()
	if err := streamBuildLogs(response.Body, func(line string) { t.Log(line) }); err != nil {
		t.Fatalf("ImageBuild stream %s: %v", ref, err)
	}
	return ref
}

func cleanupIntegrationArtifacts(ctx context.Context, driver *Driver) error {
	label := integrationLabelPrefix + ".provider=docker"
	containers, err := driver.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", label)),
	})
	if err != nil {
		return fmt.Errorf("listing docker test containers: %w", err)
	}
	for _, item := range containers {
		err := driver.cli.ContainerRemove(ctx, item.ID, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
		if err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("removing docker test container %s: %w", item.ID, err)
		}
	}

	images, err := driver.cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("listing docker test images: %w", err)
	}
	for _, item := range images {
		for _, ref := range item.RepoTags {
			if !isIntegrationImageRef(ref) {
				continue
			}
			_, err := driver.cli.ImageRemove(ctx, ref, image.RemoveOptions{Force: true, PruneChildren: false})
			if err != nil && !errdefs.IsNotFound(err) {
				return fmt.Errorf("removing docker test image %s: %w", ref, err)
			}
		}
	}
	return nil
}

func isIntegrationImageRef(ref string) bool {
	return strings.HasPrefix(ref, "hivy-docker-test-") || strings.HasPrefix(ref, "hivy-template:integration-")
}

func integrationRuntimeDockerfile() string {
	return `FROM redis:7-alpine
ENTRYPOINT []
RUN mkdir -p /www && echo ok > /www/index.html
EXPOSE 8080
CMD ["/bin/sh", "-lc", "while true; do printf 'HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nok\n' | nc -l -p 8080; done"]
`
}

func assertHTTPBody(t *testing.T, ctx context.Context, url string, want string) {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatalf("ReadAll: %v", readErr)
			}
			if resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == want {
				return
			}
			lastErr = fmt.Errorf("status=%d body=%q", resp.StatusCode, strings.TrimSpace(string(body)))
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("GET %s did not return %q: %v", url, want, lastErr)
}
