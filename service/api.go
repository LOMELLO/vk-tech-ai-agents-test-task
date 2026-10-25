package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	errNotFound        = "not_found"
	errRateLimited     = "rate_limited"
	errRequestFailed   = "request_failed"
	errInvalidResponse = "invalid_response"
)

type FetchResult struct {
	Issues       []Issue
	Error        string
	Unauthorized bool
}

type pageResult struct {
	Items []apiIssue
	Next  string
}

type Fetcher struct {
	baseURL string
	token   string
	client  *http.Client
	cache   *Cache
	trace   *Trace
	sleep   func(time.Duration)
	now     func() time.Time
}

func NewFetcher(cfg Config, cache *Cache, trace *Trace) *Fetcher {
	return &Fetcher{
		baseURL: cfg.BaseURL,
		token:   cfg.Token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: cache,
		trace: trace,
		sleep: time.Sleep,
		now:   time.Now,
	}
}

func (f *Fetcher) Fetch(ctx context.Context, repo string, limit int, since *time.Time) FetchResult {
	parts := strings.SplitN(repo, "/", 2)
	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues?state=open&sort=created&direction=asc&per_page=100",
		f.baseURL, url.PathEscape(parts[0]), url.PathEscape(parts[1]))
	next := endpoint
	issues := make([]Issue, 0)
	seen := make(map[int]bool)
	for next != "" && len(issues) < limit {
		page, code, unauthorized := f.getPage(ctx, next)
		if unauthorized {
			return FetchResult{Unauthorized: true}
		}
		if code != "" {
			return FetchResult{Error: code}
		}
		for _, item := range page.Items {
			if item.PullRequest != nil {
				continue
			}
			created, err := time.Parse(time.RFC3339, item.CreatedAt)
			if err != nil {
				return FetchResult{Error: errInvalidResponse}
			}
			if since != nil && created.Before(*since) {
				continue
			}
			if seen[item.Number] {
				continue
			}
			seen[item.Number] = true
			labels := make([]string, 0, len(item.Labels))
			for _, label := range item.Labels {
				labels = append(labels, label.Name)
			}
			issues = append(issues, Issue{
				Number:    item.Number,
				Title:     item.Title,
				Author:    item.User.Login,
				Labels:    labels,
				CreatedAt: item.CreatedAt,
				URL:       item.HTMLURL,
			})
			if len(issues) == limit {
				break
			}
		}
		next = page.Next
	}
	sort.SliceStable(issues, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339, issues[i].CreatedAt)
		right, _ := time.Parse(time.RFC3339, issues[j].CreatedAt)
		if !left.Equal(right) {
			return left.Before(right)
		}
		return issues[i].Number < issues[j].Number
	})
	return FetchResult{Issues: issues}
}

func (f *Fetcher) getPage(ctx context.Context, endpoint string) (pageResult, string, bool) {
	cached, hasCache := f.cache.Load(endpoint)
	transientRetries := 0
	rateRetries := 0
	attempt := 0
	for {
		attempt++
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return pageResult{}, errRequestFailed, false
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "ghreport/1.0")
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		if f.token != "" {
			req.Header.Set("Authorization", "Bearer "+f.token)
		}
		if hasCache && cached.ETag != "" {
			req.Header.Set("If-None-Match", cached.ETag)
		}
		resp, err := f.client.Do(req)
		if err != nil {
			message := "network"
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				message = "timeout"
			}
			_ = f.trace.Write(endpoint, 0, attempt, false, &message)
			if transientRetries < 3 {
				f.sleep(time.Duration(1<<transientRetries) * time.Second)
				transientRetries++
				continue
			}
			return pageResult{}, errRequestFailed, false
		}
		status := resp.StatusCode
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			message := "network"
			var netErr net.Error
			if errors.As(readErr, &netErr) && netErr.Timeout() {
				message = "timeout"
			}
			_ = f.trace.Write(endpoint, status, attempt, false, &message)
			if transientRetries < 3 {
				f.sleep(time.Duration(1<<transientRetries) * time.Second)
				transientRetries++
				continue
			}
			return pageResult{}, errRequestFailed, false
		}
		fromCache := status == http.StatusNotModified && hasCache
		_ = f.trace.Write(endpoint, status, attempt, fromCache, nil)
		switch {
		case status == http.StatusNotModified:
			if !hasCache {
				return pageResult{}, errRequestFailed, false
			}
			page, code := parsePage([]byte(cached.Body), cached.Link, endpoint)
			return page, code, false
		case status == http.StatusUnauthorized:
			return pageResult{}, "", true
		case status == http.StatusNotFound:
			return pageResult{}, errNotFound, false
		case status == http.StatusOK:
			link := resp.Header.Get("Link")
			_ = f.cache.Store(endpoint, resp.Header.Get("ETag"), body, link)
			page, code := parsePage(body, link, endpoint)
			return page, code, false
		case status == http.StatusForbidden || status == http.StatusTooManyRequests:
			if !isRateLimited(resp, body) {
				return pageResult{}, errRequestFailed, false
			}
			if rateRetries < 3 {
				f.sleep(f.rateLimitWait(resp))
				rateRetries++
				continue
			}
			return pageResult{}, errRateLimited, false
		case status >= 500:
			if transientRetries < 3 {
				f.sleep(time.Duration(1<<transientRetries) * time.Second)
				transientRetries++
				continue
			}
			return pageResult{}, errRequestFailed, false
		default:
			return pageResult{}, errRequestFailed, false
		}
	}
}

func parsePage(body []byte, linkHeader, current string) (pageResult, string) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return pageResult{}, errInvalidResponse
	}
	var items []apiIssue
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return pageResult{}, errInvalidResponse
	}
	return pageResult{Items: items, Next: nextLink(linkHeader, current)}, ""
}

func nextLink(header, current string) string {
	if header == "" {
		return ""
	}
	parts := splitLinkHeader(header)
	for _, part := range parts {
		fields := strings.Split(part, ";")
		if len(fields) < 2 {
			continue
		}
		target := strings.TrimSpace(fields[0])
		if !strings.HasPrefix(target, "<") || !strings.HasSuffix(target, ">") {
			continue
		}
		target = strings.TrimSuffix(strings.TrimPrefix(target, "<"), ">")
		for _, field := range fields[1:] {
			field = strings.TrimSpace(field)
			if field == `rel="next"` || field == "rel=next" {
				parsed, err := url.Parse(target)
				if err != nil {
					return ""
				}
				if parsed.IsAbs() {
					return target
				}
				base, err := url.Parse(current)
				if err != nil {
					return ""
				}
				return base.ResolveReference(parsed).String()
			}
		}
	}
	return ""
}

func splitLinkHeader(header string) []string {
	var parts []string
	start := 0
	inAngle := false
	for i, r := range header {
		switch r {
		case '<':
			inAngle = true
		case '>':
			inAngle = false
		case ',':
			if !inAngle {
				parts = append(parts, header[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, header[start:])
	return parts
}

func isRateLimited(resp *http.Response, body []byte) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "rate_limit") ||
		strings.Contains(lower, "secondary rate") ||
		strings.Contains(lower, "abuse")
}

func (f *Fetcher) rateLimitWait(resp *http.Response) time.Duration {
	if value := strings.TrimSpace(resp.Header.Get("Retry-After")); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			if seconds < 0 {
				return 0
			}
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(value); err == nil {
			wait := when.Sub(f.now())
			if wait < 0 {
				return 0
			}
			return wait
		}
	}
	if value := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")); value != "" {
		if epoch, err := strconv.ParseInt(value, 10, 64); err == nil {
			wait := time.Unix(epoch, 0).Sub(f.now())
			if wait < 0 {
				return 0
			}
			return wait
		}
	}
	return 60 * time.Second
}
