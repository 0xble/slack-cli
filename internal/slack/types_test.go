package slack

import (
	"encoding/json"
	"testing"
)

func TestHistoryResponse_MessageChannelSupportsStringOrObject(t *testing.T) {
	t.Run("channel as string", func(t *testing.T) {
		raw := []byte(`{
			"ok": true,
			"messages": [
				{
					"type": "message",
					"user": "U123",
					"text": "hello",
					"ts": "123.456",
					"channel": "C123"
				}
			]
		}`)

		var history HistoryResponse
		if err := json.Unmarshal(raw, &history); err != nil {
			t.Fatalf("json.Unmarshal returned error: %v", err)
		}

		if len(history.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(history.Messages))
		}
		if history.Messages[0].Channel == nil {
			t.Fatalf("expected channel ref to be present")
		}
		if history.Messages[0].Channel.ID != "C123" {
			t.Fatalf("expected channel ID C123, got %q", history.Messages[0].Channel.ID)
		}
	})

	t.Run("channel as object", func(t *testing.T) {
		raw := []byte(`{
			"ok": true,
			"messages": [
				{
					"type": "message",
					"user": "U123",
					"text": "hello",
					"ts": "123.456",
					"channel": {
						"id": "C123",
						"name": "general"
					}
				}
			]
		}`)

		var history HistoryResponse
		if err := json.Unmarshal(raw, &history); err != nil {
			t.Fatalf("json.Unmarshal returned error: %v", err)
		}

		if len(history.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(history.Messages))
		}
		if history.Messages[0].Channel == nil {
			t.Fatalf("expected channel ref to be present")
		}
		if history.Messages[0].Channel.ID != "C123" {
			t.Fatalf("expected channel ID C123, got %q", history.Messages[0].Channel.ID)
		}
		if history.Messages[0].Channel.Name != "general" {
			t.Fatalf("expected channel name general, got %q", history.Messages[0].Channel.Name)
		}
	})
}
