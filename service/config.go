package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.github.com"

type Config struct {
	Repos    []string
	Limit    int
	Since    *time.Time
	Out      string
	Trace    string
	CacheDir string
	Token    string
	BaseURL  string
}

func parseConfig(args []string, getenv func(string) string) (Config, error) {
	fs := flag.NewFlagSet("ghreport", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	repos := fs.String("repos", "", "")
	limit := fs.Int("limit", 100, "")
	since := fs.String("since", "", "")
	out := fs.String("out", "-", "")
	trace := fs.String("trace", "", "")
	cacheDir := fs.String("cache-dir", "./.ghreport-cache", "")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, errors.New("лишние аргументы")
	}
	if *repos == "" {
		return Config{}, errors.New("--repos обязателен")
	}
	if *limit < 1 {
		return Config{}, errors.New("--limit должен быть целым числом, не меньшим единицы")
	}
	seen := make(map[string]bool)
	list := make([]string, 0)
	for _, raw := range strings.Split(*repos, ",") {
		repo := strings.TrimSpace(raw)
		if !validRepo(repo) {
			return Config{}, fmt.Errorf("некорректный репозиторий: %q", repo)
		}
		if !seen[repo] {
			seen[repo] = true
			list = append(list, repo)
		}
	}
	if len(list) == 0 {
		return Config{}, errors.New("--repos не содержит репозиториев")
	}
	var sinceTime *time.Time
	if *since != "" {
		parsed, err := time.Parse("2006-01-02", *since)
		if err != nil {
			return Config{}, errors.New("--since должен иметь формат YYYY-MM-DD")
		}
		utc := parsed.UTC()
		sinceTime = &utc
	}
	base := strings.TrimSpace(getenv("GITHUB_API_BASE_URL"))
	if base == "" {
		base = defaultBaseURL
	}
	base = strings.TrimRight(base, "/")
	parsedBase, err := url.Parse(base)
	if err != nil || (parsedBase.Scheme != "http" && parsedBase.Scheme != "https") || parsedBase.Host == "" {
		return Config{}, errors.New("некорректный GITHUB_API_BASE_URL")
	}
	return Config{
		Repos:    list,
		Limit:    *limit,
		Since:    sinceTime,
		Out:      *out,
		Trace:    *trace,
		CacheDir: *cacheDir,
		Token:    getenv("GITHUB_TOKEN"),
		BaseURL:  base,
	}, nil
}

func validRepo(repo string) bool {
	if repo == "" || strings.ContainsAny(repo, " \t\r\n") {
		return false
	}
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return false
	}
	return parts[0] != "" && parts[1] != ""
}
