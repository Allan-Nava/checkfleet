// Pushing a run somewhere over HTTP: chat webhooks, Telegram, and the
// dead-man's-switch ping.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// pingDeadman pings a dead-man's-switch URL (Healthchecks.io-style): the base
// URL on success, base+"/fail" when the worst finding is BAD/ERROR.
func pingDeadman(ctx context.Context, url string, worst engine.Status) error {
	if worst == engine.BAD || worst == engine.ERROR {
		url = strings.TrimRight(url, "/") + "/fail"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// postRendered renders a chat payload and POSTs it to the webhook URL taken
// from env var webhookEnv, printing a confirmation. Shared by discord/teams.
func postRendered(webhookEnv, name string, render func() (string, error)) error {
	payload, err := render()
	if err != nil {
		return err
	}
	url := os.Getenv(webhookEnv)
	if url == "" {
		return fmt.Errorf("%s webhook not set: env %s is empty", name, webhookEnv)
	}
	if err := postJSON(context.Background(), url, payload); err != nil {
		return err
	}
	fmt.Printf("checkfleet: report sent to %s\n", name)
	return nil
}

// postTelegram sends a plain-text message via the Telegram Bot API sendMessage.
func postTelegram(ctx context.Context, token, chatID, text string) error {
	payload, err := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	if err != nil {
		return err
	}
	url := "https://api.telegram.org/bot" + token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending to Telegram: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram responded HTTP %d", resp.StatusCode)
	}
	return nil
}

// postJSON POSTs a JSON payload to a webhook URL, accepting any 2xx response.
func postJSON(ctx context.Context, url, payload string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending to the webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("the webhook responded HTTP %d", resp.StatusCode)
	}
	return nil
}
