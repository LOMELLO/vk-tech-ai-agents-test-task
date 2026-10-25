package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixedClock() time.Time {
	return time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
}

func testFetcher(t *testing.T, base, token string) (*Fetcher, *Cache, *bytes.Buffer) {
	t.Helper()
	cache := NewCache(t.TempDir())
	buffer := &bytes.Buffer{}
	fetcher := NewFetcher(Config{BaseURL: base, Token: token}, cache, NewTrace(buffer, fixedClock))
	fetcher.now = fixedClock
	return fetcher, cache, buffer
}

func expectNumbers(t *testing.T, issues []Issue, expected ...int) {
	t.Helper()
	actual := make([]int, 0, len(issues))
	for _, issue := range issues {
		actual = append(actual, issue.Number)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("numbers = %v, want %v", actual, expected)
	}
}

func TestFetchPaginationFilteringAndDeduplication(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/repos/o/r/issues" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if query.Get("page") == "2" {
			fmt.Fprint(w, `[
				{"number":3,"title":"three","created_at":"2026-01-03T00:00:00Z","html_url":"u3","user":{"login":"c"},"labels":[]},
				{"number":4,"title":"four","created_at":"2026-01-04T00:00:00Z","html_url":"u4","user":{"login":"d"},"labels":[]},
				{"number":5,"title":"five","created_at":"2026-01-05T00:00:00Z","html_url":"u5","user":{"login":"e"},"labels":[]}
			]`)
			return
		}
		if query.Get("state") != "open" || query.Get("sort") != "created" ||
			query.Get("direction") != "asc" || query.Get("per_page") != "100" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/issues?page=2>; rel="next"`, server.URL))
		fmt.Fprint(w, `[
			{"number":1,"title":"one","created_at":"2026-01-01T00:00:00Z","html_url":"u1","user":{"login":"a"},"labels":[{"name":"bug"}]},
			{"number":2,"title":"pull","created_at":"2026-01-02T00:00:00Z","html_url":"u2","user":{"login":"b"},"labels":[],"pull_request":{}},
			{"number":3,"title":"three","created_at":"2026-01-03T00:00:00Z","html_url":"u3","user":{"login":"c"},"labels":[]}
		]`)
	}))
	defer server.Close()

	fetcher, _, _ := testFetcher(t, server.URL, "")
	result := fetcher.Fetch(context.Background(), "o/r", 3, nil)
	if result.Error != "" || result.Unauthorized {
		t.Fatalf("result = %#v", result)
	}
	expectNumbers(t, result.Issues, 1, 3, 4)
	if result.Issues[0].Labels[0] != "bug" || result.Issues[0].Author != "a" {
		t.Fatalf("issue = %#v", result.Issues[0])
	}
}

func TestFetchSortingAndSince(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"number":2,"title":"two","created_at":"2026-01-02T00:00:00Z","html_url":"u2","user":{"login":"b"},"labels":[]},
			{"number":1,"title":"one","created_at":"2026-01-03T00:00:00Z","html_url":"u1","user":{"login":"a"},"labels":[]},
			{"number":3,"title":"three","created_at":"2026-01-01T00:00:00Z","html_url":"u3","user":{"login":"c"},"labels":[]}
		]`)
	}))
	defer server.Close()

	fetcher, _, _ := testFetcher(t, server.URL, "")
	since := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	result := fetcher.Fetch(context.Background(), "o/r", 10, &since)
	if result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	expectNumbers(t, result.Issues, 2, 1)
}

func TestCacheConditionalRequests(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprint(w, `[{"number":1,"title":"one","created_at":"2026-01-01T00:00:00Z","html_url":"u1","user":{"login":"a"},"labels":[]}]`)
	}))
	defer server.Close()

	fetcher1, cache, trace1 := testFetcher(t, server.URL, "top-secret")
	first := fetcher1.Fetch(context.Background(), "o/r", 10, nil)
	if first.Error != "" {
		t.Fatalf("first = %#v", first)
	}
	fetcher2 := NewFetcher(Config{BaseURL: server.URL}, cache, NewTrace(&bytes.Buffer{}, fixedClock))
	fetcher2.now = fixedClock
	trace2 := &bytes.Buffer{}
	fetcher2.trace = NewTrace(trace2, fixedClock)
	second := fetcher2.Fetch(context.Background(), "o/r", 10, nil)
	if second.Error != "" || !reflect.DeepEqual(first.Issues, second.Issues) {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
	if !strings.Contains(trace1.String(), `"status":200`) || !strings.Contains(trace1.String(), `"from_cache":false`) {
		t.Fatalf("trace1 = %s", trace1.String())
	}
	if !strings.Contains(trace2.String(), `"status":304`) || !strings.Contains(trace2.String(), `"from_cache":true`) {
		t.Fatalf("trace2 = %s", trace2.String())
	}
	entries, err := os.ReadDir(cache.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(cache.dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "top-secret") {
			t.Fatal("token found in cache")
		}
	}
}

func TestRateLimitRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
			return
		}
		fmt.Fprint(w, `[{"number":1,"title":"one","created_at":"2026-01-01T00:00:00Z","html_url":"u1","user":{"login":"a"},"labels":[]}]`)
	}))
	defer server.Close()

	fetcher, _, _ := testFetcher(t, server.URL, "")
	var sleeps []time.Duration
	fetcher.sleep = func(d time.Duration) { sleeps = append(sleeps, d) }
	result := fetcher.Fetch(context.Background(), "o/r", 10, nil)
	if result.Error != "" {
		t.Fatalf("result = %#v", result)
	}
	if requests != 2 || len(sleeps) != 1 || sleeps[0] != 0 {
		t.Fatalf("requests=%d sleeps=%v", requests, sleeps)
	}
}

func TestRateLimitExhausted(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	fetcher, _, _ := testFetcher(t, server.URL, "")
	fetcher.sleep = func(time.Duration) {}
	result := fetcher.Fetch(context.Background(), "o/r", 10, nil)
	if result.Error != errRateLimited || requests != 4 {
		t.Fatalf("result=%#v requests=%d", result, requests)
	}
}

func TestServerErrorRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	fetcher, _, _ := testFetcher(t, server.URL, "")
	var sleeps []time.Duration
	fetcher.sleep = func(d time.Duration) { sleeps = append(sleeps, d) }
	result := fetcher.Fetch(context.Background(), "o/r", 10, nil)
	if result.Error != "" || requests != 2 {
		t.Fatalf("result=%#v requests=%d", result, requests)
	}
	if !reflect.DeepEqual(sleeps, []time.Duration{time.Second}) {
		t.Fatalf("sleeps = %v", sleeps)
	}
}

func TestFetchErrors(t *testing.T) {
	cases := []struct {
		status       int
		body         string
		wantError    string
		unauthorized bool
	}{
		{http.StatusNotFound, `{}`, errNotFound, false},
		{http.StatusUnauthorized, `{}`, "", true},
		{http.StatusOK, `{"not":"an array"}`, errInvalidResponse, false},
		{http.StatusOK, `[{]`, errInvalidResponse, false},
	}
	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		fetcher, _, _ := testFetcher(t, server.URL, "")
		result := fetcher.Fetch(context.Background(), "o/r", 10, nil)
		server.Close()
		if result.Error != tc.wantError || result.Unauthorized != tc.unauthorized {
			t.Fatalf("status=%d result=%#v", tc.status, result)
		}
	}
}

func TestNextLink(t *testing.T) {
	current := "https://api.example/repos/o/r/issues?per_page=100"
	header := `<https://api.example/repos/o/r/issues?page=2>; rel="next", <https://api.example/repos/o/r/issues?page=7>; rel="last"`
	if got := nextLink(header, current); got != "https://api.example/repos/o/r/issues?page=2" {
		t.Fatalf("next = %q", got)
	}
	if got := nextLink(`</repos/o/r/issues?page=2>; rel="next"`, current); got != "https://api.example/repos/o/r/issues?page=2" {
		t.Fatalf("relative next = %q", got)
	}
	if got := nextLink(`<https://api.example/x>; rel="last"`, current); got != "" {
		t.Fatalf("next = %q", got)
	}
}
