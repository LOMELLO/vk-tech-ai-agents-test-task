package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/good/repo/issues":
			fmt.Fprint(w, `[{"number":1,"title":"one","created_at":"2026-01-01T00:00:00Z","html_url":"u1","user":{"login":"a"},"labels":[{"name":"bug"}]}]`)
		case "/repos/missing/repo/issues":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"Not Found"}`)
		case "/repos/auth/repo/issues":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Bad credentials"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testEnv(serverURL, token string) func(string) string {
	return func(key string) string {
		switch key {
		case "GITHUB_API_BASE_URL":
			return serverURL
		case "GITHUB_TOKEN":
			return token
		default:
			return ""
		}
	}
}

func TestRunSuccessWithTraceAndFileOutput(t *testing.T) {
	server := testServer(t)
	defer server.Close()
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "trace.jsonl")
	outPath := filepath.Join(dir, "report.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--repos", "good/repo", "--cache-dir", filepath.Join(dir, "cache"), "--trace", tracePath, "--out", outPath},
		&stdout, &stderr, testEnv(server.URL, "super-secret"), fixedClock)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.GeneratedAt != "2026-10-14T09:00:00Z" || len(report.Repos) != 1 {
		t.Fatalf("report = %#v", report)
	}
	repo := report.Repos[0]
	if repo.Error != nil || repo.Count != 1 || len(repo.Issues) != 1 || repo.Issues[0].Author != "a" {
		t.Fatalf("repo = %#v", repo)
	}
	traceData, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	traceText := string(traceData)
	if strings.Contains(traceText, "super-secret") || strings.Contains(traceText, "Authorization") {
		t.Fatalf("trace leaks authorization: %s", traceText)
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(traceText)), &entry); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ts", "method", "url", "status", "attempt", "from_cache", "error"} {
		if _, ok := entry[key]; !ok {
			t.Fatalf("missing trace field %s in %v", key, entry)
		}
	}
}

func TestRunPartialFailure(t *testing.T) {
	server := testServer(t)
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := run([]string{"--repos", "good/repo,missing/repo", "--cache-dir", t.TempDir()},
		&stdout, &stderr, testEnv(server.URL, ""), fixedClock)
	if code != 1 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Repos[1].Error == nil || *report.Repos[1].Error != errNotFound || report.Repos[1].Count != 0 {
		t.Fatalf("report = %#v", report)
	}
	if !strings.Contains(stdout.String(), `"issues":[]`) {
		t.Fatalf("issues must be empty array: %s", stdout.String())
	}
}

func TestRunAllFailed(t *testing.T) {
	server := testServer(t)
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := run([]string{"--repos", "missing/repo", "--cache-dir", t.TempDir()},
		&stdout, &stderr, testEnv(server.URL, ""), fixedClock)
	if code != 3 || stdout.String() != "{\"error\": \"all_failed\"}\n" {
		t.Fatalf("code=%d stdout=%q", code, stdout.String())
	}
}

func TestRunUnauthorized(t *testing.T) {
	server := testServer(t)
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := run([]string{"--repos", "auth/repo", "--cache-dir", t.TempDir()},
		&stdout, &stderr, testEnv(server.URL, ""), fixedClock)
	if code != 3 || stdout.String() != "{\"error\": \"unauthorized\"}\n" {
		t.Fatalf("code=%d stdout=%q", code, stdout.String())
	}
}

func TestRunInvalidArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--repos", "a/b", "--limit", "0"}, &stdout, &stderr,
		func(string) string { return "" }, fixedClock)
	if code != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
