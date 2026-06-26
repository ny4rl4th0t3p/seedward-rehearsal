// Command rehearse is the standalone one-shot rehearsal runner: it reads a launch's local
// inputs (a gentool config plus the allocation CSVs and gentxs it names), boots an ephemeral
// chain on substitute validators, runs the assertion suite, and reports a structured verdict.
// It talks to no coordd — a chain team drops it into CI to self-rehearse before submitting.
//
// Exit codes mirror the engine's tri-state: 0 = PASS, 1 = FAIL, 2 = ERROR.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/config"
	"github.com/ny4rl4th0t3p/seedward-gentool/pkg/rehearse"
)

func main() {
	cfgPath := flag.String("config", "", "path to the gentool YAML config (required)")
	binary := flag.String("binary", "", "path to the chaind binary (required)")
	binarySHA := flag.String("binary-sha256", "", "expected sha256 of the binary (hex); empty skips the digest check")
	validators := flag.Int("validators", 2, "number of substitute validators to boot")
	bootWait := flag.Duration("boot-wait", 90*time.Second, "max wait for the chain's first block")
	jsonOut := flag.Bool("json", false, "emit the result as JSON instead of a human-readable report")
	flag.Parse()

	if *cfgPath == "" || *binary == "" {
		fmt.Fprintln(os.Stderr, "both --config and --binary are required")
		flag.Usage()
		os.Exit(int(exitError))
	}

	in, err := buildInput(*cfgPath, *binary, *binarySHA)
	if err != nil {
		fmt.Fprintln(os.Stderr, "input error:", err)
		os.Exit(int(exitError))
	}

	engine := rehearse.New(
		rehearse.NewProcessRuntime(*binary),
		rehearse.WithValidators(*validators),
		rehearse.WithBootWait(*bootWait),
	)
	res, err := engine.Run(context.Background(), in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "engine error:", err)
		os.Exit(int(exitError))
	}

	if *jsonOut {
		emitJSON(res)
	} else {
		fmt.Print(res.Report())
	}
	os.Exit(int(exitCode(res.Outcome)))
}

// exit codes mirror the engine's tri-state outcome.
const (
	exitPass  = 0
	exitFail  = 1
	exitError = 2
)

func exitCode(o rehearse.Outcome) int {
	switch o {
	case rehearse.OutcomePass:
		return exitPass
	case rehearse.OutcomeFail:
		return exitFail
	default:
		return exitError
	}
}

// buildInput resolves a gentool config file (via gentool's own loader) and the local files it
// names into the engine's Input. The binary path/digest come from flags, not the config.
func buildInput(cfgPath, binary, sha string) (rehearse.Input, error) {
	inputs, err := config.Load(cfgPath)
	if err != nil {
		return rehearse.Input{}, err
	}

	gentxs, err := readGentxs(inputs.GentxDir)
	if err != nil {
		return rehearse.Input{}, err
	}

	alloc := map[rehearse.AllocationType][]byte{}
	for _, a := range []struct {
		typ  rehearse.AllocationType
		path string
	}{
		{rehearse.AllocationAccounts, inputs.Accounts},
		{rehearse.AllocationClaims, inputs.Claims},
		{rehearse.AllocationGrants, inputs.Grants},
		{rehearse.AllocationAuthz, inputs.Authz},
		{rehearse.AllocationFeegrant, inputs.Feegrant},
	} {
		if a.path == "" {
			continue
		}
		b, err := os.ReadFile(a.path)
		if err != nil {
			return rehearse.Input{}, fmt.Errorf("read %s allocation %s: %w", a.typ, a.path, err)
		}
		alloc[a.typ] = b
	}

	return rehearse.Input{
		Config:       inputs.Chain,
		Allocations:  alloc,
		Gentxs:       gentxs,
		BinaryPath:   binary,
		BinarySHA256: sha,
	}, nil
}

// readGentxs reads every *.json file in dir as a raw gentx.
func readGentxs(dir string) ([][]byte, error) {
	if dir == "" {
		return nil, fmt.Errorf("validators.gentx_dir is not set in the config")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read gentx dir %s: %w", dir, err)
	}
	var gentxs [][]byte
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read gentx %s: %w", e.Name(), err)
		}
		gentxs = append(gentxs, b)
	}
	if len(gentxs) == 0 {
		return nil, fmt.Errorf("no gentx .json files found in %s", dir)
	}
	return gentxs, nil
}

// emitJSON prints a machine-readable result (for the Action's outputs / CI consumption).
func emitJSON(res *rehearse.Result) {
	type step struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail,omitempty"`
	}
	out := struct {
		Outcome    string `json:"outcome"`
		FailedStep string `json:"failed_step,omitempty"`
		Summary    string `json:"summary"`
		Validators int    `json:"validators"`
		Steps      []step `json:"steps"`
	}{
		Outcome:    string(res.Outcome),
		FailedStep: res.FailedStep,
		Summary:    res.Summary,
		Validators: res.Validators,
	}
	for _, s := range res.Steps {
		out.Steps = append(out.Steps, step{Name: s.Name, Status: string(s.Status), Detail: s.Detail})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal result:", err)
		return
	}
	fmt.Println(string(b))
}
