package receipt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
)

type Handler struct {
	sender Sender
	mu     sync.Mutex
	seen   map[string]Decision
}

func NewHandler(sender Sender) *Handler {
	return &Handler{sender: sender, seen: make(map[string]Decision)}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method must be POST"})
		return
	}
	var event BuildEvent
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid build event"})
		return
	}

	decision, err := h.process(r.Context(), event)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Error()})
			return
		}
		if errors.As(err, &apiErr) {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "email delivery was not accepted"})
			return
		}
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationErr.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "email delivery could not be completed"})
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

func (h *Handler) process(ctx context.Context, event BuildEvent) (Decision, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if prior, ok := h.seen[event.EventID]; ok {
		prior.Reason = "event already processed"
		return prior, nil
	}
	decision, err := Decide(event)
	if err != nil {
		return Decision{}, err
	}
	if decision.Action == "send" {
		decision.MessageID, err = h.sender.Send(ctx, ReceiptEmail(event), "build-event-"+event.EventID)
		if err != nil {
			return Decision{}, err
		}
		decision.Action = "sent"
	}
	h.seen[event.EventID] = decision
	return decision, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
