package main

import (
	"reflect"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	env := func(key string) string {
		switch key {
		case "GITHUB_API_BASE_URL":
			return "http://example.test/api/"
		case "GITHUB_TOKEN":
			return "secret"
		default:
			return ""
		}
	}
	cfg, err := parseConfig([]string{"--repos", "a/b, a/b,c/d", "--limit", "5", "--since", "2026-01-02"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Repos, []string{"a/b", "c/d"}) {
		t.Fatalf("repos = %#v", cfg.Repos)
	}
	if cfg.Limit != 5 || cfg.BaseURL != "http://example.test/api" || cfg.Token != "secret" {
		t.Fatalf("config = %#v", cfg)
	}
	if cfg.Since == nil || !cfg.Since.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("since = %#v", cfg.Since)
	}
}

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig([]string{"--repos", "a/b"}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != defaultBaseURL || cfg.Limit != 100 || cfg.Out != "-" || cfg.CacheDir != "./.ghreport-cache" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"--repos", ""},
		{"--repos", "a/b", "--limit", "0"},
		{"--repos", "a/b", "--since", "2026-13-01"},
		{"--repos", "a/b/c"},
		{"--repos", "a/"},
		{"--repos", "a b/c"},
		{"--repos", "a/b", "extra"},
		{"--repos", "a/b", "--unknown"},
	}
	for _, args := range cases {
		if _, err := parseConfig(args, func(string) string { return "" }); err == nil {
			t.Fatalf("expected error for %#v", args)
		}
	}
}
