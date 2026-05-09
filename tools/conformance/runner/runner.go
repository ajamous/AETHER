// Command conformance-runner aggregates Aether's per-module unit
// tests into a single conformance-suite invocation.
//
// SGP.23 testing in real labs uses a battery of test cases driven
// against a black-box SM-DP+. The lab's harness fronts the
// system; ours can't. Instead, this runner walks the tests we
// already have, classifies them by the SGP.23 family they
// exercise (per tools/conformance/coverage/sgp23.md), and reports
// pass/fail + coverage as a single human-readable summary.
//
// The runner does NOT redefine tests; it shells out to `go test
// -count=1 -run <pattern> <module>` for each declared test in
// the catalogue. Adding a test to the conformance suite is a
// matter of appending a row in catalogue.go, not duplicating
// test logic.
//
// Usage:
//
//	go run ./tools/conformance/runner
//	go run ./tools/conformance/runner -v
//	go run ./tools/conformance/runner -family ES9+
//
// CI invokes this on every PR. `make conformance` from the repo
// root is the local entry point.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	verbose := flag.Bool("v", false, "stream test output as it runs")
	familyFilter := flag.String("family", "", "run only the named family (e.g. ES9+, ES12, ES8+)")
	listOnly := flag.Bool("list", false, "list the catalogue and exit, don't run anything")
	flag.Parse()

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "conformance-runner:", err)
		os.Exit(2)
	}

	cases := catalogue
	if *familyFilter != "" {
		cases = filterFamily(cases, *familyFilter)
		if len(cases) == 0 {
			fmt.Fprintf(os.Stderr, "conformance-runner: no cases for family %q\n", *familyFilter)
			os.Exit(2)
		}
	}

	if *listOnly {
		printCatalogue(cases)
		return
	}

	results := run(repoRoot, cases, *verbose)
	exitCode := report(results)
	os.Exit(exitCode)
}

// run executes each case via `go test`. Cases sharing a (module,
// package, run-pattern) tuple still get individual rows in the
// report but are run together by go test for efficiency.
func run(repoRoot string, cases []Case, verbose bool) []Result {
	results := make([]Result, 0, len(cases))
	for _, c := range cases {
		modPath := filepath.Join(repoRoot, c.Module)
		args := []string{"test", "-count=1", "-run", c.RunPattern, c.Package}
		cmd := exec.Command("go", args...)
		cmd.Dir = modPath
		start := time.Now()
		out, err := cmd.CombinedOutput()
		dur := time.Since(start)

		r := Result{Case: c, Duration: dur, OK: err == nil}
		if err != nil {
			r.Output = string(out)
		} else if verbose {
			r.Output = string(out)
		}
		results = append(results, r)
	}
	return results
}

func report(results []Result) int {
	// Group by family for the summary.
	byFamily := map[string][]Result{}
	familyKeys := []string{}
	totalPass, totalFail := 0, 0
	totalDur := time.Duration(0)
	for _, r := range results {
		if _, ok := byFamily[r.Case.Family]; !ok {
			familyKeys = append(familyKeys, r.Case.Family)
		}
		byFamily[r.Case.Family] = append(byFamily[r.Case.Family], r)
		totalDur += r.Duration
		if r.OK {
			totalPass++
		} else {
			totalFail++
		}
	}
	sort.Strings(familyKeys)

	fmt.Println("== SGP.23 conformance suite ==")
	fmt.Println()
	for _, fam := range familyKeys {
		rows := byFamily[fam]
		passed := 0
		for _, r := range rows {
			if r.OK {
				passed++
			}
		}
		fmt.Printf("[%s] %d/%d passed\n", fam, passed, len(rows))
		for _, r := range rows {
			mark := "PASS"
			if !r.OK {
				mark = "FAIL"
			}
			fmt.Printf("    %s %-50s (%s)\n", mark, r.Case.Title, r.Duration.Round(time.Millisecond))
			if !r.OK {
				out := strings.TrimRight(r.Output, "\n")
				for _, line := range strings.Split(out, "\n") {
					fmt.Printf("        %s\n", line)
				}
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d passed, %d failed in %s\n",
		totalPass, totalFail, totalDur.Round(time.Millisecond))

	if totalFail > 0 {
		return 1
	}
	return 0
}

func printCatalogue(cases []Case) {
	families := map[string][]Case{}
	keys := []string{}
	for _, c := range cases {
		if _, ok := families[c.Family]; !ok {
			keys = append(keys, c.Family)
		}
		families[c.Family] = append(families[c.Family], c)
	}
	sort.Strings(keys)
	for _, fam := range keys {
		fmt.Printf("[%s]\n", fam)
		for _, c := range families[fam] {
			fmt.Printf("    %s\n", c.Title)
			fmt.Printf("        %s :: %s :: %s\n", c.Module, c.Package, c.RunPattern)
		}
	}
}

func filterFamily(cases []Case, family string) []Case {
	out := make([]Case, 0, len(cases))
	for _, c := range cases {
		if c.Family == family {
			out = append(out, c)
		}
	}
	return out
}

// findRepoRoot walks upward looking for the workspace root
// (where go.work lives). Lets `go run ./tools/conformance/runner`
// work no matter where the user runs it from.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.work found from %s upward", dir)
		}
		dir = parent
	}
}
