package main

import "encoding/json"

type Report struct {
	GeneratedAt string       `json:"generated_at"`
	Repos       []RepoReport `json:"repos"`
}

type RepoReport struct {
	Repo   string  `json:"repo"`
	Error  *string `json:"error"`
	Count  int     `json:"count"`
	Issues []Issue `json:"issues"`
}

type Issue struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	Labels    []string `json:"labels"`
	CreatedAt string   `json:"created_at"`
	URL       string   `json:"url"`
}

type apiIssue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	HTMLURL   string `json:"html_url"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest json.RawMessage `json:"pull_request"`
}
