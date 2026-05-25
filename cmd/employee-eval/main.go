package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/evals"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "employee-eval: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var suitePath string
	var modelsCSV string
	var runs int
	var parallel int
	var apiURL string
	var outDir string
	var judgeModel string
	flag.StringVar(&suitePath, "suite", "evals/employee-delegation-v1.yaml", "eval suite YAML path")
	flag.StringVar(&modelsCSV, "models", "", "comma-separated model ids; defaults to suite models")
	flag.IntVar(&runs, "runs", 1, "number of runs per model/case")
	flag.IntVar(&parallel, "parallel", 1, "maximum concurrent trials")
	flag.StringVar(&apiURL, "api-url", "http://localhost:8080", "local control-plane API URL")
	flag.StringVar(&outDir, "out", "", "artifact output directory")
	flag.StringVar(&judgeModel, "judge-model", evals.DefaultJudgeModel, "model used for nondeterministic eval judgement")
	flag.Parse()

	suite, err := evals.LoadSuite(suitePath)
	if err != nil {
		return err
	}
	if outDir == "" {
		stamp := time.Now().UTC().Format("20060102T150405Z") + "-" + uuid.NewString()[:8]
		outDir = filepath.Join("tmp", "evals", "runs", stamp)
	}
	ctx := context.Background()
	deps, err := bootstrap.New(ctx)
	if err != nil {
		return err
	}
	defer deps.Close(ctx)

	opts := evals.RunOptions{
		SuitePath:  suitePath,
		Models:     splitCSV(modelsCSV),
		Runs:       runs,
		Parallel:   parallel,
		APIURL:     apiURL,
		OutDir:     outDir,
		JudgeModel: judgeModel,
	}
	summary, runErr := evals.NewRunner(deps).Run(ctx, suite, opts)
	if summary != nil {
		if err := evals.WriteArtifacts(outDir, suite, summary, deps.DB); err != nil && runErr == nil {
			runErr = err
		}
		fmt.Printf("eval artifacts: %s\n", outDir)
		fmt.Printf("overall pass rate: %.1f%% (%d/%d)\n",
			summary.Overall.PassRate,
			summary.Overall.Passed,
			summary.Overall.TotalCases,
		)
	}
	return runErr
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
