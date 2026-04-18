// Package output provides stdout formatters for human, markdown, and
// machine-readable (JSON / JSONL) renderings.
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
