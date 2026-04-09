package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pixell07/canopy/internal/models"
	"github.com/pixell07/canopy/internal/repository"
	"go.uber.org/zap"
)

const (
	maxRetries      = 3
	retryBackoff    = 2 * time.Second
	deliveryTimeout = 10 * time.Second
)

// WebhookPayload is the envelope sent to all webhook targets.
type WebhookPayload struct {
	Event     models.WebhookEvent    `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// WebhookNotifier fetches active webhooks from the DB and delivers payloads
// with HMAC-SHA256 signatures and exponential retry.
type WebhookNotifier struct {
	webhookRepo *repository.WebhookRepo
	client      *http.Client
	log         *zap.Logger
}

func NewWebhookNotifier(wr *repository.WebhookRepo, log *zap.Logger) *WebhookNotifier {
	return &WebhookNotifier{
		webhookRepo: wr,
		client:      &http.Client{Timeout: deliveryTimeout},
		log:         log,
	}
}

// Send delivers the event to all registered active webhooks for that event type.
// Safe to call in a goroutine — it recovers from panics.
func (n *WebhookNotifier) Send(ctx context.Context, event models.WebhookEvent, data map[string]interface{}) {
	defer func() {
		if r := recover(); r != nil {
			n.log.Error("webhook notifier panic", zap.Any("recover", r))
		}
	}()

	targets, err := n.webhookRepo.GetActiveForEvent(ctx, event)
	if err != nil {
		n.log.Error("webhook: failed to fetch targets", zap.String("event", string(event)), zap.Error(err))
		return
	}

	if len(targets) == 0 {
		return
	}

	payload := WebhookPayload{
		Event:     event,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		n.log.Error("webhook: failed to marshal payload", zap.Error(err))
		return
	}

	for _, wh := range targets {
		n.deliver(ctx, wh, body)
	}
}

func (n *WebhookNotifier) deliver(ctx context.Context, wh *models.Webhook, body []byte) {
	sig := signPayload(body, wh.Secret)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
		if err != nil {
			n.log.Error("webhook: failed to build request", zap.String("url", wh.URL), zap.Error(err))
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Canopy-Signature", "sha256="+sig)
		req.Header.Set("X-Canopy-Event", string(wh.Name))
		req.Header.Set("User-Agent", "canopy-webhooks/1.0")

		resp, err := n.client.Do(req)
		if err != nil {
			lastErr = err
			n.log.Warn("webhook: delivery failed",
				zap.String("webhook", wh.Name),
				zap.String("url", wh.URL),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
			time.Sleep(retryBackoff * time.Duration(attempt))
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			n.log.Info("webhook: delivered",
				zap.String("webhook", wh.Name),
				zap.Int("status", resp.StatusCode),
				zap.Int("attempt", attempt),
			)
			return
		}

		lastErr = fmt.Errorf("non-2xx status: %d", resp.StatusCode)
		n.log.Warn("webhook: non-2xx response",
			zap.String("webhook", wh.Name),
			zap.Int("status", resp.StatusCode),
			zap.Int("attempt", attempt),
		)
		time.Sleep(retryBackoff * time.Duration(attempt))
	}

	n.log.Error("webhook: all retries exhausted",
		zap.String("webhook", wh.Name),
		zap.String("url", wh.URL),
		zap.Error(lastErr),
	)
}

// signPayload creates an HMAC-SHA256 signature of the body using the webhook secret.
// Consumers verify: sha256=<hex> matches X-Canopy-Signature header.
func signPayload(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
