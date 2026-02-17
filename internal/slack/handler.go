package slack

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type MentionHandler interface {
	HandleMention(event Event)
	HandleThreadMessage(event Event)
	HandleSlashCommand(command, text, channel, user, responseURL string)
}

type Event struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Text     string `json:"text"`
	Channel  string `json:"channel"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts,omitempty"` // Thread timestamp (if in a thread)
}

type Handler struct {
	api             *slack.Client
	socketClient    *socketmode.Client
	mentionHandler  MentionHandler
	processedEvents sync.Map
}

func NewHandler(appToken, botToken string, mentionHandler MentionHandler) *Handler {
	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
	socketClient := socketmode.New(api)

	h := &Handler{
		api:            api,
		socketClient:   socketClient,
		mentionHandler: mentionHandler,
	}
	go h.cleanupLoop()
	return h
}

func (h *Handler) SetMentionHandler(mh MentionHandler) {
	h.mentionHandler = mh
}

func (h *Handler) APIClient() *slack.Client {
	return h.api
}

func (h *Handler) Run(ctx context.Context) error {
	go h.handleEvents(ctx)
	return h.socketClient.RunContext(ctx)
}

func (h *Handler) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-h.socketClient.Events:
			if !ok {
				return
			}
			h.processEvent(evt)
		}
	}
}

func (h *Handler) processEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		h.socketClient.Ack(*evt.Request)

		if eventsAPIEvent.Type == slackevents.CallbackEvent {
			// Extract EventID from the callback event data
			var eventID string
			if cb, ok := eventsAPIEvent.Data.(*slackevents.EventsAPICallbackEvent); ok {
				eventID = cb.EventID
			}

			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				// Dedup by event ID
				if eventID == "" {
					eventID = ev.TimeStamp // fallback
				}
				if _, loaded := h.processedEvents.LoadOrStore(eventID, time.Now()); loaded {
					slog.Info("duplicate event, skipping", "event_id", eventID)
					return
				}

				slog.Info("received app_mention",
					"event_id", eventID,
					"channel", ev.Channel,
					"user", ev.User,
				)

				event := Event{
					Type:     ev.Type,
					User:     ev.User,
					Text:     ev.Text,
					Channel:  ev.Channel,
					TS:       ev.TimeStamp,
					ThreadTS: ev.ThreadTimeStamp, // Thread timestamp for replies
				}
				go h.mentionHandler.HandleMention(event)

			case *slackevents.MessageEvent:
				// Only handle thread messages (not bot messages)
				if ev.BotID != "" {
					return // Ignore bot messages
				}

				// Dedup by event ID
				if eventID == "" {
					eventID = ev.TimeStamp // fallback
				}
				if _, loaded := h.processedEvents.LoadOrStore(eventID, time.Now()); loaded {
					return
				}

				// Only process messages in threads
				if ev.ThreadTimeStamp != "" {
					slog.Info("received thread_message",
						"event_id", eventID,
						"channel", ev.Channel,
						"user", ev.User,
						"thread_ts", ev.ThreadTimeStamp,
					)

					event := Event{
						Type:     ev.Type,
						User:     ev.User,
						Text:     ev.Text,
						Channel:  ev.Channel,
						TS:       ev.TimeStamp,
						ThreadTS: ev.ThreadTimeStamp,
					}
					go h.mentionHandler.HandleThreadMessage(event)
				}
			}
		}

	case socketmode.EventTypeSlashCommand:
		cmd, ok := evt.Data.(slack.SlashCommand)
		if !ok {
			return
		}
		h.socketClient.Ack(*evt.Request)

		slog.Info("received slash_command",
			"command", cmd.Command,
			"channel", cmd.ChannelID,
			"user", cmd.UserID,
		)

		go h.mentionHandler.HandleSlashCommand(
			cmd.Command,
			cmd.Text,
			cmd.ChannelID,
			cmd.UserID,
			cmd.ResponseURL,
		)
	}
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
