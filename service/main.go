package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv, time.Now))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string, clock func() time.Time) int {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "ghreport: %v\n", err)
		return 2
	}
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "ghreport: не удалось создать каталог кэша: %v\n", err)
		return 2
	}
	var traceWriter io.Writer
	if cfg.Trace != "" {
		file, err := os.Create(cfg.Trace)
		if err != nil {
			fmt.Fprintf(stderr, "ghreport: не удалось открыть журнал: %v\n", err)
			return 2
		}
		defer file.Close()
		traceWriter = file
	}
	trace := NewTrace(traceWriter, clock)
	fetcher := NewFetcher(cfg, NewCache(cfg.CacheDir), trace)
	fetcher.now = clock

	report := Report{GeneratedAt: clock().UTC().Format(time.RFC3339), Repos: make([]RepoReport, 0, len(cfg.Repos))}
	failed := 0
	for _, repo := range cfg.Repos {
		result := fetcher.Fetch(context.Background(), repo, cfg.Limit, cfg.Since)
		if result.Unauthorized {
			writeFatal(stdout, "unauthorized")
			return 3
		}
		repoReport := RepoReport{Repo: repo, Issues: []Issue{}}
		if result.Error != "" {
			code := result.Error
			repoReport.Error = &code
			failed++
		} else {
			repoReport.Issues = result.Issues
			if repoReport.Issues == nil {
				repoReport.Issues = []Issue{}
			}
			repoReport.Count = len(repoReport.Issues)
		}
		report.Repos = append(report.Repos, repoReport)
	}
	if failed == len(cfg.Repos) {
		writeFatal(stdout, "all_failed")
		return 3
	}
	if err := writeReport(report, cfg.Out, stdout); err != nil {
		fmt.Fprintf(stderr, "ghreport: не удалось записать отчёт: %v\n", err)
		return 2
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func writeFatal(stdout io.Writer, code string) {
	_, _ = fmt.Fprintf(stdout, "{\"error\": %q}\n", code)
}

func writeReport(report Report, out string, stdout io.Writer) error {
	var writer io.Writer = stdout
	var file *os.File
	if out != "-" {
		var err error
		file, err = os.Create(out)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(report)
}
