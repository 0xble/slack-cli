package slack

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DateFilter captures a resolved time window for commands that accept
// --after / --before / --on / --last flags. The zero value means
// "no filter".
type DateFilter struct {
	After  time.Time
	Before time.Time
}

// DateFilterFlags is the shared kong flag block for commands that accept
// --after / --before / --on / --last. Embed it into a command struct to
// avoid duplicating the flag definitions, then call Resolve to produce a
// DateFilter.
type DateFilterFlags struct {
	After  string `help:"Only match messages on or after DATE (YYYY-MM-DD, UTC)" xor:"after-last,after-on"`
	Before string `help:"Only match messages on or before DATE (YYYY-MM-DD, UTC)" xor:"before-on"`
	On     string `help:"Only match messages on DATE (YYYY-MM-DD, UTC)" xor:"after-on,before-on,on-last"`
	Last   string `help:"Only match messages from the last DURATION (e.g. 45d, 12h, 2w)" xor:"after-last,on-last"`
}

// Resolve validates the embedded flags and returns a filter anchored at now.
func (f DateFilterFlags) Resolve(now time.Time) (DateFilter, error) {
	return ResolveDateFilter(f.After, f.Before, f.On, f.Last, now)
}

// IsZero returns true when neither bound is set.
func (d DateFilter) IsZero() bool {
	return d.After.IsZero() && d.Before.IsZero()
}

// ResolveDateFilter validates the flag combination and returns a filter
// anchored at now.
//
//   - after and before are YYYY-MM-DD (UTC midnight for After, end-of-day for
//     Before).
//   - on is YYYY-MM-DD, expanded to the full day.
//   - last is a duration ending at now. Go's time.ParseDuration units plus
//     d (24h) and w (7d) are accepted.
//
// Mutually exclusive: on with any other flag; last with after.
func ResolveDateFilter(after, before, on, last string, now time.Time) (DateFilter, error) {
	after = strings.TrimSpace(after)
	before = strings.TrimSpace(before)
	on = strings.TrimSpace(on)
	last = strings.TrimSpace(last)

	if on != "" && (after != "" || before != "" || last != "") {
		return DateFilter{}, fmt.Errorf("--on cannot be combined with --after, --before, or --last")
	}
	if last != "" && after != "" {
		return DateFilter{}, fmt.Errorf("--last cannot be combined with --after")
	}

	var f DateFilter

	if on != "" {
		day, err := parseDateStart(on)
		if err != nil {
			return DateFilter{}, fmt.Errorf("--on: %w", err)
		}
		f.After = day
		f.Before = day.Add(24*time.Hour - time.Nanosecond)
		return f, nil
	}

	if after != "" {
		t, err := parseDateStart(after)
		if err != nil {
			return DateFilter{}, fmt.Errorf("--after: %w", err)
		}
		f.After = t
	}
	if before != "" {
		t, err := parseDateStart(before)
		if err != nil {
			return DateFilter{}, fmt.Errorf("--before: %w", err)
		}
		// Inclusive upper bound: end of that day.
		f.Before = t.Add(24*time.Hour - time.Nanosecond)
	}
	if last != "" {
		d, err := parseExtendedDuration(last)
		if err != nil {
			return DateFilter{}, fmt.Errorf("--last: %w", err)
		}
		if d <= 0 {
			return DateFilter{}, fmt.Errorf("--last: duration must be positive")
		}
		f.After = now.Add(-d)
	}

	if !f.After.IsZero() && !f.Before.IsZero() && f.Before.Before(f.After) {
		return DateFilter{}, fmt.Errorf("--before is earlier than --after")
	}

	return f, nil
}

// ToTimestampParams converts the filter into (oldest, latest) Slack
// timestamp strings suitable for conversations.history / conversations.replies.
// Empty string means "unset". Timestamps are emitted with microsecond
// precision so an inclusive end-of-day bound like 23:59:59.999999999 does
// not get truncated to the start of the last second.
func (d DateFilter) ToTimestampParams() (oldest, latest string) {
	if !d.After.IsZero() {
		oldest = slackTimestamp(d.After)
	}
	if !d.Before.IsZero() {
		latest = slackTimestamp(d.Before)
	}
	return oldest, latest
}

// ToSearchOperators converts the filter into Slack search-query operators
// (after:YYYY-MM-DD, before:YYYY-MM-DD). Returns empty string when unset.
// Slack's search operators take calendar dates, not timestamps.
func (d DateFilter) ToSearchOperators() string {
	var parts []string
	if !d.After.IsZero() {
		// Slack's after: operator is exclusive, so subtract a day to get
		// an inclusive lower bound that matches the flag's documented semantics.
		parts = append(parts, "after:"+d.After.Add(-24*time.Hour).UTC().Format("2006-01-02"))
	}
	if !d.Before.IsZero() {
		// before: is also exclusive; add a day for inclusivity.
		parts = append(parts, "before:"+d.Before.Add(24*time.Hour).UTC().Format("2006-01-02"))
	}
	return strings.Join(parts, " ")
}

// ValidateSearchLast rejects sub-day --last durations, which Slack's search
// operators cannot express (they are calendar-date only). Callers of search
// should invoke this before composing the query so a flag like --last 12h
// fails loudly instead of silently broadening to a multi-day window.
func ValidateSearchLast(last string) error {
	last = strings.TrimSpace(last)
	if last == "" {
		return nil
	}
	dur, err := parseExtendedDuration(last)
	if err != nil {
		// Bad input is surfaced by ResolveDateFilter; no reason to
		// double-report here.
		return nil
	}
	if dur < 24*time.Hour {
		return fmt.Errorf("search only supports day-precision windows; --last durations shorter than 24h cannot be expressed as a Slack search operator")
	}
	return nil
}

// QueryHasDateOperator reports whether the given search query already
// contains an after:, before:, on:, or during: operator. Used by the search
// command to reject silent overrides when the user passes both a query
// operator and a flag.
var dateOperatorPattern = regexp.MustCompile(`(?i)(^|\s)(after|before|on|during):\S`)

func QueryHasDateOperator(query string) bool {
	return dateOperatorPattern.MatchString(query)
}

func parseDateStart(s string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected YYYY-MM-DD, got %q", s)
	}
	return t, nil
}

// parseExtendedDuration extends time.ParseDuration with d (24h) and w (7d).
func parseExtendedDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	// Pull out any terminal d/w suffix and convert to hours before handing
	// off to time.ParseDuration.
	if len(s) > 1 {
		last := s[len(s)-1]
		if last == 'd' || last == 'w' {
			numStr := s[:len(s)-1]
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid duration %q", s)
			}
			var hours float64
			switch last {
			case 'd':
				hours = n * 24
			case 'w':
				hours = n * 24 * 7
			}
			return time.Duration(hours * float64(time.Hour)), nil
		}
	}

	return time.ParseDuration(s)
}

// slackTimestamp formats t as a Slack seconds.microseconds string so
// fractional bounds (e.g. 23:59:59.999999 for an inclusive end-of-day)
// survive round-tripping through conversations.history params.
func slackTimestamp(t time.Time) string {
	micros := t.Nanosecond() / 1000
	return fmt.Sprintf("%d.%06d", t.Unix(), micros)
}
