package slack

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

func VerifyRequest(signingSecret string, r *http.Request) error {
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	if timestamp == "" {
		return fmt.Errorf("missing X-Slack-Request-Timestamp header")
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if math.Abs(float64(time.Now().Unix()-ts)) > 300 {
		return fmt.Errorf("request timestamp too old")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	sigBaseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(sigBaseString))
	expected := fmt.Sprintf("v0=%x", mac.Sum(nil))

	actual := r.Header.Get("X-Slack-Signature")
	if !hmac.Equal([]byte(expected), []byte(actual)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}
