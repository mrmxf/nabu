// Package mail sends Nabu's email (sign-in codes, verification and password
// resets) through a provider's HTTP API instead of SMTP. Providers are small and
// pluggable: each turns a PocketBase *mailer.Message into one API call. Add one
// by writing a constructor and listing it in providers.
//
// Bind wires a Sender into PocketBase's OnMailerSend hook, so every message the
// app sends goes through the provider and PocketBase's own mailer never runs.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/tools/mailer"
)

// Sender delivers one message.
type Sender interface {
	// Send delivers m, or returns why it could not.
	Send(ctx context.Context, m *mailer.Message) error
	// Name is the provider name, for logs.
	Name() string
}

// Config configures a provider. APIKey is the secret itself, read from the env
// var the course config names; it is never logged.
type Config struct {
	Provider string
	APIKey   string
	// Endpoint overrides the provider's default API URL.
	Endpoint string
	// From, when its Address is set, replaces the sender of every message.
	From mail.Address
	// Client is the HTTP client; nil means one with a 15 s timeout.
	Client *http.Client
}

type factory func(Config) Sender

// providers lists every provider by its config name.
var providers = map[string]factory{
	"brevo":    newBrevo,
	"mailtrap": newMailtrap,
}

// Providers returns the provider names, sorted.
func Providers() []string {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Known reports whether name is a provider.
func Known(name string) bool {
	_, ok := providers[strings.ToLower(name)]
	return ok
}

// New returns the Sender for c.Provider.
func New(c Config) (Sender, error) {
	f, ok := providers[strings.ToLower(c.Provider)]
	if !ok {
		return nil, fmt.Errorf("mail: unknown provider %q: use one of %s", c.Provider, strings.Join(Providers(), ", "))
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("mail: provider %s has no API key", c.Provider)
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 15 * time.Second}
	}
	return f(c), nil
}

// from is the sender of m: c.From when configured, else the message's own.
func (c Config) from(m *mailer.Message) mail.Address {
	if c.From.Address != "" {
		return c.From
	}
	return m.From
}

// postJSON POSTs body as JSON to url with headers, and fails on a non-2xx reply,
// quoting the start of the provider's error body (it never holds our key).
func postJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("mail: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("mail: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mail: post %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("mail: %s answered %d: %s", url, resp.StatusCode, strings.TrimSpace(string(msg)))
}

// ErrNoRecipient is returned for a message with no To address.
var ErrNoRecipient = errors.New("mail: message has no recipient")
