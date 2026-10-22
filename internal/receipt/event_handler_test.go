package receipt

import (
	"context"
	"testing"
)

type recordingSender struct {
	calls []SendRequest
	keys  []string
}

func (s *recordingSender) Send(_ context.Context, request SendRequest, key string) (string, error) {
	s.calls = append(s.calls, request)
	s.keys = append(s.keys, key)
	return "msg_42", nil
}

func TestReleaseDecision(t *testing.T) {
	tests := []struct {
		name       string
		event      BuildEvent
		wantAction string
		wantCalls  int
	}{
		{name: "successful release sends receipt", event: BuildEvent{EventID: "evt-42", Kind: "release", Status: "succeeded", ReleaseID: "rel-9", Repository: "acme/compiler", CommitSHA: "7ac91ef", Recipient: "dev@example.com", AmountCents: 2400}, wantAction: "sent", wantCalls: 1},
		{name: "failed release is retained without email", event: BuildEvent{EventID: "evt-43", Kind: "release", Status: "failed"}, wantAction: "skipped", wantCalls: 0},
		{name: "ordinary build is retained without email", event: BuildEvent{EventID: "evt-44", Kind: "build", Status: "succeeded"}, wantAction: "skipped", wantCalls: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &recordingSender{}
			handler := NewHandler(sender)
			got, err := handler.process(context.Background(), tt.event)
			if err != nil {
				t.Fatal(err)
			}
			if got.Action != tt.wantAction || len(sender.calls) != tt.wantCalls {
				t.Fatalf("action=%q calls=%d, want action=%q calls=%d", got.Action, len(sender.calls), tt.wantAction, tt.wantCalls)
			}
		})
	}
}

func TestDuplicateEventSendsOnce(t *testing.T) {
	sender := &recordingSender{}
	handler := NewHandler(sender)
	event := BuildEvent{EventID: "evt-42", Kind: "release", Status: "succeeded", ReleaseID: "rel-9", Repository: "acme/compiler", CommitSHA: "7ac91ef", Recipient: "dev@example.com", AmountCents: 2400}
	for i := 0; i < 2; i++ {
		if _, err := handler.process(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.calls) != 1 {
		t.Fatalf("send calls=%d, want 1", len(sender.calls))
	}
	if sender.keys[0] != "build-event-evt-42" {
		t.Fatalf("idempotency key=%q", sender.keys[0])
	}
}
