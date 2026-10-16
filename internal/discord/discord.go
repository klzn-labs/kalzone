package discord

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/adrianozp/platform/pkg/config"
)

const timeout = 10 * time.Second

type message struct {
	Content         string   `json:"content"`
	AllowedMentions mentions `json:"allowed_mentions"`
}

type mentions struct {
	Parse []string `json:"parse"`
}

type Notifier struct {
	url    string
	client *http.Client
}

func NewNotifier(cfg config.Config) Notifier {
	return Notifier{url: cfg.Approvals.WebhookURL, client: &http.Client{Timeout: timeout}}
}

func (n Notifier) Notify(text string) {
	if n.url == "" {
		return
	}
	go n.send(text)
}

func (n Notifier) send(text string) {
	if err := n.post(text); err != nil {
		log.Printf("discord notification not sent: %v", withoutURL(err))
	}
}

func (n Notifier) post(text string) error {
	body, err := json.Marshal(message{Content: text, AllowedMentions: mentions{Parse: []string{}}})
	if err != nil {
		return err
	}
	resp, err := n.client.Post(n.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("the webhook answered %s", resp.Status)
	}
	return nil
}

func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
