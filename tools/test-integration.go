// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build ignore

// Runs the top-level tests of a compiled integration test binary, each in its own process, on several
// workers at once: each worker is a GITEA_TEST_INSTANCE with its own work path, database and port.
// The slowest tests of the last run start first, so a long one does not start last.
//
//	go run tools/test-integration.go [-p N] [-run REGEXP] BINARY
package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type result struct {
	name   string
	passed bool
	secs   float64
	output []byte
}

func main() {
	parallel := flag.Int("p", max(1, runtime.NumCPU()*3/4), "number of tests to run at once") // fastest when measured on 16 CPUs: 8 or 16 workers took 60 s and 53 s, 12 took 50 s
	run := flag.String("run", "", "run only the top-level tests matching this regular expression")
	timeout := flag.String("timeout", "30m", "timeout of each test")
	verbose := flag.Bool("v", false, "print the output of passing tests too")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run tools/test-integration.go [-p N] [-run REGEXP] BINARY")
		os.Exit(2)
	}
	binary, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		fail(err)
	}
	filter, err := regexp.Compile(*run)
	if err != nil {
		fail(err)
	}

	listOut, err := exec.Command(binary, "-test.list", "^Test").Output()
	if err != nil {
		fail(fmt.Errorf("list tests: %w", err))
	}
	var names []string
	for name := range strings.FieldsSeq(string(listOut)) {
		if filter.MatchString(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		fail(fmt.Errorf("no tests match %q", *run))
	}

	// the tests run in tests/integration, as go test runs them
	dir, err := filepath.Abs(filepath.Join("tests", "integration"))
	if err != nil {
		fail(err)
	}
	if _, err := os.Stat(dir); err != nil {
		fail(fmt.Errorf("run this from the source root: %w", err))
	}
	timesFile := filepath.Join(dir, "gitea-integration-times.json")
	times := map[string]float64{}
	if data, err := os.ReadFile(timesFile); err == nil {
		_ = json.Unmarshal(data, &times)
	}
	slices.SortStableFunc(names, func(a, b string) int { return cmp.Compare(times[b], times[a]) })

	workers := min(*parallel, len(names))
	fmt.Printf("running %d tests on %d workers\n", len(names), workers)
	start := time.Now()
	queue := make(chan string)
	results := make(chan result)
	var wg sync.WaitGroup
	for instance := 1; instance <= workers; instance++ {
		wg.Go(func() {
			for name := range queue {
				results <- runTest(binary, dir, instance, name, *timeout)
			}
		})
	}
	go func() {
		for _, name := range names {
			queue <- name
		}
		close(queue)
		wg.Wait()
		close(results)
	}()

	var failed []string
	done := 0
	for r := range results {
		done++
		times[r.name] = r.secs
		status := "ok  "
		if !r.passed {
			status = "FAIL"
			failed = append(failed, r.name)
		}
		fmt.Printf("%s %-60s %6.2fs  [%d/%d]\n", status, r.name, r.secs, done, len(names))
		if !r.passed || *verbose {
			os.Stdout.Write(r.output)
		}
	}
	if data, err := json.MarshalIndent(times, "", "  "); err == nil {
		_ = os.WriteFile(timesFile, data, 0o644)
	}

	fmt.Printf("\n%d passed, %d failed in %s\n", len(names)-len(failed), len(failed), time.Since(start).Round(time.Second))
	if len(failed) > 0 {
		fmt.Printf("failed: %s\n", strings.Join(failed, " "))
		os.Exit(1)
	}
}

func runTest(binary, dir string, instance int, name, timeout string) result {
	cmd := exec.Command(binary, "-test.run", "^"+regexp.QuoteMeta(name)+"$", "-test.timeout", timeout)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GITEA_TEST_INSTANCE="+strconv.Itoa(instance))
	if os.Getenv("GITEA_TEST_DATABASE") == "" {
		cmd.Env = append(cmd.Env, "GITEA_TEST_DATABASE=sqlite")
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	err := cmd.Run()
	secs := time.Since(start).Seconds()
	// a test that matches nothing prints a warning and passes, so check that it ran
	passed := err == nil && !bytes.Contains(out.Bytes(), []byte("no tests to run"))
	return result{name: name, passed: passed, secs: secs, output: out.Bytes()}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
