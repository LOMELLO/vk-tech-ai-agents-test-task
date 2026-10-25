package main

import (
	"encoding/json"
	"io"
	"time"
)

type Trace struct {
	encoder *json.Encoder
	clock   func() time.Time
}

type traceEntry struct {
	Timestamp string  `json:"ts"`
	Method    string  `json:"method"`
	URL       string  `json:"url"`
	Status    int     `json:"status"`
	Attempt   int     `json:"attempt"`
	FromCache bool    `json:"from_cache"`
	Error     *string `json:"error"`
}

func NewTrace(writer io.Writer, clock func() time.Time) *Trace {
	if writer == nil {
		return nil
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return &Trace{encoder: encoder, clock: clock}
}

func (t *Trace) Write(url string, status, attempt int, fromCache bool, errText *string) error {
	if t == nil {
		return nil
	}
	return t.encoder.Encode(traceEntry{
		Timestamp: t.clock().UTC().Format(time.RFC3339),
		Method:    "GET",
		URL:       url,
		Status:    status,
		Attempt:   attempt,
		FromCache: fromCache,
		Error:     errText,
	})
}
