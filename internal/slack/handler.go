package slack

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type MentionHandler interface {
	HandleMention(event Event)
}

type Event struct {
	Type    string `json:"type"`
	User    string `json:"user"`
	Text    string `json:"text"`
	Channel string `json:"channel"`
	TS      string `json:"ts"`
}

type slackRequest struct {
	Token     string `json:"token"`
	Challenge string `json:"challenge"`
	Type      string `json:"type"`
	EventID   string `json:"event_id"`
	Event     Event  `json:"event"`
}

type Handler struct {
	signingSecret   string
	mentionHandler  MentionHandler
	processedEvents sync.Map
}

func NewHandler(signingSecret string, mentionHandler MentionHandler) *Handler {
	h := &Handler{
		signingSecret:  signingSecret,
		mentionHandler: mentionHandler,
	}
	// Periodically clean up old event IDs
	go h.cleanupLoop()
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := VerifyRequest(h.signingSecret, r); err != nil {
		slog.Warn("signature verification failed", "error", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var req slackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// URL verification challenge
	if req.Type == "url_verification" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"challenge": req.Challenge})
		return
	}

	// Event callback
	if req.Type == "event_callback" && req.Event.Type == "app_mention" {
		// Dedup by event_id
		if _, loaded := h.processedEvents.LoadOrStore(req.EventID, time.Now()); loaded {
			slog.Info("duplicate event, skipping", "event_id", req.EventID)
			w.WriteHeader(http.StatusOK)
			return
		}

		slog.Info("received app_mention",
			"event_id", req.EventID,
			"channel", req.Event.Channel,
			"user", req.Event.User,
		)

		// Respond immediately, process async
		w.WriteHeader(http.StatusOK)
		go h.mentionHandler.HandleMention(req.Event)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-30 * time.Minute)
		h.processedEvents.Range(func(key, value any) bool {
			if t, ok := value.(time.Time); ok && t.Before(cutoff) {
				h.processedEvents.Delete(key)
			}
			return true
		})
	}
}
