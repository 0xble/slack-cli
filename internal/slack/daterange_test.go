package slack

import (
	"strings"
	"testing"
	"time"
)

var refNow = time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

func TestResolveDateFilter_After(t *testing.T) {
	f, err := ResolveDateFilter("2026-04-01", "", "", "", refNow)
	if err != nil {
		t.Fatalf("ResolveDateFilter returned error: %v", err)
	}
	want := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if !f.After.Equal(want) {
		t.Fatalf("expected After=%v, got %v", want, f.After)
	}
	if !f.Before.IsZero() {
		t.Fatalf("expected zero Before, got %v", f.Before)
	}
}

func TestResolveDateFilter_Before(t *testing.T) {
	f, err := ResolveDateFilter("", "2026-04-15", "", "", refNow)
	if err != nil {
		t.Fatalf("ResolveDateFilter returned error: %v", err)
	}
	// Inclusive: end of day.
	if !f.Before.After(time.Date(2026, 4, 15, 23, 59, 0, 0, time.UTC)) {
		t.Fatalf("expected Before to be end of day, got %v", f.Before)
	}
	if f.Before.Day() != 15 || f.Before.Month() != 4 {
		t.Fatalf("expected Before on 2026-04-15, got %v", f.Before)
	}
}

func TestResolveDateFilter_On(t *testing.T) {
	f, err := ResolveDateFilter("", "", "2026-04-10", "", refNow)
	if err != nil {
		t.Fatalf("ResolveDateFilter returned error: %v", err)
	}
	if f.After.Day() != 10 || !f.Before.After(f.After) {
		t.Fatalf("expected single-day window for 2026-04-10, got %+v", f)
	}
}

func TestResolveDateFilter_Last(t *testing.T) {
	f, err := ResolveDateFilter("", "", "", "7d", refNow)
	if err != nil {
		t.Fatalf("ResolveDateFilter returned error: %v", err)
	}
	want := refNow.Add(-7 * 24 * time.Hour)
	if !f.After.Equal(want) {
		t.Fatalf("expected After=%v, got %v", want, f.After)
	}
	if !f.Before.IsZero() {
		t.Fatalf("expected zero Before, got %v", f.Before)
	}
}

func TestResolveDateFilter_LastAndBeforeCombines(t *testing.T) {
	f, err := ResolveDateFilter("", "2026-04-15", "", "30d", refNow)
	if err != nil {
		t.Fatalf("ResolveDateFilter returned error: %v", err)
	}
	if f.After.IsZero() || f.Before.IsZero() {
		t.Fatalf("expected both bounds set, got %+v", f)
	}
}

func TestResolveDateFilter_ForbiddenCombinations(t *testing.T) {
	tests := []struct {
		name                 string
		after, before, on, last string
		wantErr              string
	}{
		{"on with after", "2026-04-01", "", "2026-04-10", "", "--on cannot"},
		{"on with before", "", "2026-04-15", "2026-04-10", "", "--on cannot"},
		{"on with last", "", "", "2026-04-10", "7d", "--on cannot"},
		{"last with after", "2026-04-01", "", "", "7d", "--last cannot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveDateFilter(tt.after, tt.before, tt.on, tt.last, refNow)
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestResolveDateFilter_InvalidDate(t *testing.T) {
	_, err := ResolveDateFilter("2026/04/01", "", "", "", refNow)
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("expected YYYY-MM-DD format error, got %v", err)
	}
}

func TestResolveDateFilter_InvalidDuration(t *testing.T) {
	_, err := ResolveDateFilter("", "", "", "abc", refNow)
	if err == nil {
		t.Fatalf("expected duration error")
	}
}

func TestResolveDateFilter_BeforeEarlierThanAfter(t *testing.T) {
	_, err := ResolveDateFilter("2026-04-20", "2026-04-10", "", "", refNow)
	if err == nil || !strings.Contains(err.Error(), "earlier than --after") {
		t.Fatalf("expected before<after error, got %v", err)
	}
}

func TestDateFilter_ToTimestampParams(t *testing.T) {
	f, err := ResolveDateFilter("2026-04-01", "2026-04-15", "", "", refNow)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	oldest, latest := f.ToTimestampParams()
	if oldest == "" || latest == "" {
		t.Fatalf("expected both timestamps set, got oldest=%q latest=%q", oldest, latest)
	}
}

func TestDateFilter_ToSearchOperators(t *testing.T) {
	f, err := ResolveDateFilter("2026-04-01", "2026-04-15", "", "", refNow)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	ops := f.ToSearchOperators()
	// Slack search after: is exclusive, so we offset by one day.
	if !strings.Contains(ops, "after:2026-03-31") {
		t.Fatalf("expected after:2026-03-31 (exclusive offset), got %q", ops)
	}
	if !strings.Contains(ops, "before:2026-04-16") {
		t.Fatalf("expected before:2026-04-16 (exclusive offset), got %q", ops)
	}
}

func TestQueryHasDateOperator(t *testing.T) {
	tests := map[string]bool{
		"deploy":                       false,
		"deploy after:2026-04-01":      true,
		"before:2026-04-10 rollback":   true,
		"alice on:yesterday":           true,
		"from:@alice during:april":     true,
		"says 'after breakfast'":       false, // 'after' without colon+token
		"mentions beforehand":          false,
		"multiword after: broken":      false, // after: with empty token should not match
	}
	for q, want := range tests {
		got := QueryHasDateOperator(q)
		if got != want {
			t.Fatalf("QueryHasDateOperator(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestParseExtendedDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"30m":  30 * time.Minute,
		"12h":  12 * time.Hour,
		"2d":   48 * time.Hour,
		"1w":   7 * 24 * time.Hour,
		"1.5d": 36 * time.Hour,
	}
	for s, want := range tests {
		got, err := parseExtendedDuration(s)
		if err != nil {
			t.Fatalf("parseExtendedDuration(%q) returned error: %v", s, err)
		}
		if got != want {
			t.Fatalf("parseExtendedDuration(%q) = %v, want %v", s, got, want)
		}
	}
}
