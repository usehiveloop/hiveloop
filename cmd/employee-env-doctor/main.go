// employee-env-doctor prints a redacted employee sandbox env report.
//
// Usage:
//
//	go run ./cmd/employee-env-doctor -id <daytona-sandbox-id>
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	toolbox "github.com/daytonaio/daytona/libs/toolbox-api-client-go"

	"github.com/usehivy/hivy/internal/employeeruntime"
)

type remoteEnvPayload struct {
	PID string            `json:"pid"`
	Env map[string]string `json:"env"`
}

type doctorOutput struct {
	SandboxID string                                   `json:"sandbox_id"`
	PID       string                                   `json:"pid"`
	Entries   []employeeruntime.EmployeeEnvReportEntry `json:"entries"`
}

func main() {
	sandboxID := flag.String("id", "", "Daytona sandbox ID")
	pid := flag.String("pid", "", "Specific process PID inside the sandbox; defaults to employee runtime process or PID 1")
	jsonOut := flag.Bool("json", true, "Emit JSON instead of a table")
	includeUnexpected := flag.Bool("include-unexpected", false, "Include env keys not listed in the employee env catalog")
	includeSensitive := flag.Bool("sensitive", false, "Print sensitive env values too")
	envFile := flag.String("env-file", ".env", "Env file to load before connecting to Daytona; set empty to disable")
	flag.Parse()

	if *sandboxID == "" {
		fmt.Fprintln(os.Stderr, "error: -id is required")
		flag.Usage()
		os.Exit(1)
	}
	if err := loadEnvFile(*envFile); err != nil {
		log.Fatalf("load env file: %v", err)
	}
	if strings.TrimSpace(os.Getenv("SANDBOX_PROVIDER_KEY")) == "" {
		log.Fatal("SANDBOX_PROVIDER_KEY is required; export it or provide an env file with -env-file")
	}
	if strings.TrimSpace(os.Getenv("SANDBOX_PROVIDER_URL")) == "" {
		log.Fatal("SANDBOX_PROVIDER_URL is required; export it or provide an env file with -env-file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := executeToolboxCommand(ctx, os.Getenv("SANDBOX_PROVIDER_URL"), os.Getenv("SANDBOX_PROVIDER_KEY"), *sandboxID, employeeEnvKeysCommand(*pid))
	if err != nil {
		log.Fatalf("exec env probe: %v", err)
	}
	if result.exitCode != 0 {
		fmt.Fprint(os.Stderr, result.output)
		os.Exit(result.exitCode)
	}

	var payload remoteEnvPayload
	if err := json.Unmarshal([]byte(result.output), &payload); err != nil {
		log.Fatalf("decode env probe output: %v\noutput:\n%s", err, result.output)
	}
	output := doctorOutput{
		SandboxID: *sandboxID,
		PID:       payload.PID,
		Entries:   employeeruntime.EmployeeEnvReportFromEnv(payload.Env, *includeUnexpected, *includeSensitive),
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(output); err != nil {
			log.Fatalf("encode output: %v", err)
		}
		return
	}
	printTable(output)
}

type commandResult struct {
	output   string
	exitCode int
}

func executeToolboxCommand(ctx context.Context, apiURL, apiKey, sandboxID, command string) (commandResult, error) {
	client, err := toolboxClient(apiURL, apiKey, sandboxID)
	if err != nil {
		return commandResult{}, err
	}
	req := toolbox.NewExecuteRequest(command)
	resp, _, err := client.ProcessAPI.ExecuteCommand(ctx).Request(*req).Execute()
	if err != nil {
		return commandResult{}, err
	}
	return commandResult{output: resp.GetResult(), exitCode: int(resp.GetExitCode())}, nil
}

func toolboxClient(apiURL, apiKey, sandboxID string) (*toolbox.APIClient, error) {
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parsing SANDBOX_PROVIDER_URL %q: %w", apiURL, err)
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

func employeeEnvKeysCommand(pid string) string {
	prefix := ""
	if pid != "" {
		prefix = "EMPLOYEE_ENV_PID=" + shellQuote(pid) + " "
	}
	return prefix + `bash -lc 'set -euo pipefail
pid="${EMPLOYEE_ENV_PID:-}"
if [ -z "$pid" ]; then
  if command -v pgrep >/dev/null 2>&1; then
    pid="$(pgrep -f "[e]mployee-bridge|[e]mployee-runtime" | head -n 1 || true)"
  fi
fi
if [ -z "$pid" ]; then
  pid=1
fi
python3 - "$pid" <<'"'"'PY'"'"'
import json
import pathlib
import sys

pid = sys.argv[1]
data = pathlib.Path(f"/proc/{pid}/environ").read_bytes()
env = {}
for part in data.split(b"\0"):
    if not part or b"=" not in part:
        continue
    key, value = part.split(b"=", 1)
    env[key.decode("utf-8", "replace")] = value.decode("utf-8", "replace")
print(json.dumps({"pid": pid, "env": env}))
PY
'`
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

func printTable(output doctorOutput) {
	fmt.Printf("sandbox_id: %s\npid: %s\n\n", output.SandboxID, output.PID)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tSOURCE\tSET\tSTATUS\tSENSITIVE\tFORBIDDEN\tVALUE")
	for _, entry := range output.Entries {
		fmt.Fprintf(w, "%s\t%s\t%t\t%s\t%t\t%t\t%s\n",
			entry.Key, entry.Source, entry.Set, entry.Status, entry.Sensitive, entry.Forbidden, entry.Value)
	}
	_ = w.Flush()
}
