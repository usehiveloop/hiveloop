// employee-debug-pack uploads and runs an employee sandbox debug collector,
// downloads the resulting archive, extracts it under /tmp, and prints the
// absolute path to every extracted file.
//
// Usage:
//
//	go run ./cmd/employee-debug-pack -id <daytona-sandbox-id>
package main

import (
	"bufio"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	toolbox "github.com/daytonaio/daytona/libs/toolbox-api-client-go"
)

//go:embed debug-pack.sh
var debugScript []byte

func main() {
	sandboxID := flag.String("id", "", "Daytona sandbox ID")
	envFile := flag.String("env-file", ".env", "Env file to load before connecting to Daytona; set empty to disable")
	localDir := flag.String("local-dir", "/tmp", "Local directory where the archive is downloaded and extracted")
	includeSensitive := flag.Bool("sensitive", false, "Include raw sensitive env values in the debug pack")
	timeout := flag.Duration("timeout", 10*time.Minute, "Overall timeout")
	flag.Parse()

	if *sandboxID == "" {
		fmt.Fprintln(os.Stderr, "error: -id is required")
		flag.Usage()
		os.Exit(1)
	}
	if err := loadEnvFile(*envFile); err != nil {
		log.Fatalf("load env file: %v", err)
	}
	apiKey := strings.TrimSpace(os.Getenv("HIVY_DAYTONA_API_KEY"))
	apiURL := strings.TrimSpace(os.Getenv("HIVY_DAYTONA_API_URL"))
	if apiKey == "" {
		log.Fatal("HIVY_DAYTONA_API_KEY is required; export it or provide an env file with -env-file")
	}
	if apiURL == "" {
		log.Fatal("HIVY_DAYTONA_API_URL is required; export it or provide an env file with -env-file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client, err := toolboxClient(apiURL, apiKey, *sandboxID)
	if err != nil {
		log.Fatalf("toolbox client: %v", err)
	}

	stamp := time.Now().UTC().Format("20060102T150405Z")
	remoteScript := fmt.Sprintf("/tmp/hivy-employee-debug-pack-%s.sh", stamp)
	remoteBase := fmt.Sprintf("/tmp/sandbox-runtime-debug-%s-%s", *sandboxID, stamp)
	remoteArchive := remoteBase + ".tar.gz"

	fmt.Printf("upload_script=%s\n", remoteScript)
	if err := uploadFile(ctx, client, remoteScript, debugScript); err != nil {
		log.Fatalf("upload debug script: %v", err)
	}

	sensitive := "0"
	if *includeSensitive {
		sensitive = "1"
	}
	runCommand := fmt.Sprintf(
		"HIVY_DEBUG_OUT=%s HIVY_DEBUG_SANDBOX_ID=%s HIVY_DEBUG_SENSITIVE=%s bash %s",
		shellQuote(remoteBase),
		shellQuote(*sandboxID),
		shellQuote(sensitive),
		shellQuote(remoteScript),
	)
	fmt.Printf("run_debug_script=%s\n", remoteScript)
	result, err := executeToolboxCommand(ctx, client, runCommand)
	if err != nil {
		log.Fatalf("run debug script: %v", err)
	}
	if result.exitCode != 0 {
		fmt.Fprint(os.Stderr, result.output)
		log.Fatalf("debug script exited with code %d", result.exitCode)
	}
	if strings.TrimSpace(result.output) != "" {
		fmt.Print(result.output)
		if !strings.HasSuffix(result.output, "\n") {
			fmt.Println()
		}
	}

	if err := os.MkdirAll(*localDir, 0o755); err != nil {
		log.Fatalf("create local dir: %v", err)
	}
	localArchive := filepath.Join(*localDir, filepath.Base(remoteArchive))
	fmt.Printf("download_archive=%s\n", remoteArchive)
	if err := downloadFile(ctx, client, remoteArchive, localArchive); err != nil {
		log.Fatalf("download archive: %v", err)
	}
	fmt.Printf("local_archive=%s\n", localArchive)

	extracted, err := extractTarGz(localArchive, *localDir)
	if err != nil {
		log.Fatalf("extract archive: %v", err)
	}
	sort.Strings(extracted)

	extractDir := filepath.Join(*localDir, strings.TrimSuffix(strings.TrimSuffix(filepath.Base(localArchive), ".gz"), ".tar"))
	fmt.Printf("extract_dir=%s\n", extractDir)
	fmt.Println("archive_contents:")
	for _, file := range extracted {
		fmt.Println(file)
	}
}

type commandResult struct {
	output   string
	exitCode int
}

func executeToolboxCommand(ctx context.Context, client *toolbox.APIClient, command string) (commandResult, error) {
	req := toolbox.NewExecuteRequest(command)
	resp, _, err := client.ProcessAPI.ExecuteCommand(ctx).Request(*req).Execute()
	if err != nil {
		return commandResult{}, err
	}
	return commandResult{output: resp.GetResult(), exitCode: int(resp.GetExitCode())}, nil
}

func uploadFile(ctx context.Context, client *toolbox.APIClient, remotePath string, data []byte) error {
	tmp, err := os.CreateTemp("", "employee-debug-script-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		return err
	}
	_, _, err = client.FileSystemAPI.UploadFile(ctx).Path(remotePath).File(tmp).Execute()
	return err
}

func downloadFile(ctx context.Context, client *toolbox.APIClient, remotePath string, localPath string) error {
	source, _, err := client.FileSystemAPI.DownloadFile(ctx).Path(remotePath).Execute()
	if err != nil {
		return err
	}
	defer source.Close()
	if _, err := source.Seek(0, 0); err != nil {
		return err
	}

	dest, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer dest.Close()
	_, err = io.Copy(dest, source)
	return err
}

func toolboxClient(apiURL, apiKey, sandboxID string) (*toolbox.APIClient, error) {
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parsing HIVY_DAYTONA_API_URL %q: %w", apiURL, err)
	}
	basePath := strings.TrimRight(parsed.Path, "/") + "/toolbox/" + sandboxID + "/toolbox"
	cfg := toolbox.NewConfiguration()
	cfg.Host = parsed.Host
	cfg.Scheme = parsed.Scheme
	cfg.Servers = toolbox.ServerConfigurations{
		{URL: fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, basePath)},
	}
	cfg.AddDefaultHeader("Authorization", "Bearer "+apiKey)
	return toolbox.NewAPIClient(cfg), nil
}

func loadEnvFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, stripEnvValueQuotes(strings.TrimSpace(value)))
	}
	return scanner.Err()
}

func stripEnvValueQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

func shellQuote(value string) string {
	out := "'"
	for _, r := range value {
		if r == '\'' {
			out += `'\''`
			continue
		}
		out += string(r)
	}
	return out + "'"
}
