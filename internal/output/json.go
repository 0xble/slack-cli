package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// EmitJSON writes v as indented JSON to stdout, followed by a trailing newline.
func EmitJSON(v any) error {
	return writeJSON(os.Stdout, v)
}

// EmitJSONL writes each item in items as a single JSON object per line to
// stdout. Compact, lossless, pipeable to `jq -c`.
func EmitJSONL[T any](items []T) error {
	return writeJSONL(os.Stdout, items)
}

// EmitJSONLStream writes one record per line by repeatedly calling next until
// it reports done (ok=false). It avoids buffering the full slice in memory,
// which matters for paginated sources. When next returns an error, that
// error is wrapped and returned.
func EmitJSONLStream[T any](next func() (T, bool, error)) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	for {
		item, ok, err := next()
		if err != nil {
			return fmt.Errorf("encoding jsonl: %w", err)
		}
		if !ok {
			return nil
		}
		if err := enc.Encode(item); err != nil {
			return fmt.Errorf("encoding jsonl: %w", err)
		}
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encoding json: %w", err)
	}
	return nil
}

func writeJSONL[T any](w io.Writer, items []T) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return fmt.Errorf("encoding jsonl: %w", err)
		}
	}
	return nil
}
