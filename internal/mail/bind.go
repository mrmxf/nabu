package mail

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/embedfs"
)

// FromConfig builds the Sender the course config asks for. It returns (nil, nil)
// when mail.provider is empty: PocketBase's own mailer stays in charge.
func FromConfig(cfg embedfs.MailConfig) (Sender, error) {
	if cfg.Provider == "" {
		return nil, nil
	}
	if !Known(cfg.Provider) {
		return nil, fmt.Errorf("mail: unknown mail.provider %q: use one of %v", cfg.Provider, Providers())
	}
	key := os.Getenv(cfg.APIKeyEnv)
	if cfg.APIKeyEnv == "" || key == "" {
		return nil, fmt.Errorf("mail: provider %s needs its API key in env var %q (mail.api_key_env)", cfg.Provider, cfg.APIKeyEnv)
	}
	return New(Config{
		Provider: cfg.Provider,
		APIKey:   key,
		Endpoint: cfg.Endpoint,
		From:     mail.Address{Name: cfg.SenderName, Address: cfg.SenderAddress},
	})
}

// Bind routes every email PocketBase sends through the provider named in the
// course config. The sender is built on first use, so commands that never send
// mail never need the key. When mail.provider is empty the hook passes the
// message on to PocketBase's own mailer.
func Bind(app core.App) {
	var (
		once   sync.Once
		sender Sender
		err    error
	)
	app.OnMailerSend().BindFunc(func(e *core.MailerEvent) error {
		once.Do(func() {
			cfg, cerr := embedfs.LoadConfig()
			if cerr != nil {
				err = cerr
				return
			}
			sender, err = FromConfig(cfg.Mail)
		})
		if err != nil {
			return err
		}
		if sender == nil {
			return e.Next()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if serr := sender.Send(ctx, e.Message); serr != nil {
			slog.Warn("mail not sent", "provider", sender.Name(), "subject", e.Message.Subject, "err", serr)
			return serr
		}
		slog.Info("mail sent", "provider", sender.Name(), "subject", e.Message.Subject, "to", len(e.Message.To))
		return nil
	})
}

// Report logs, at serve start, how mail will be sent: Info when ready, Error
// with the fix when the configured provider cannot work.
func Report() {
	cfg, err := embedfs.LoadConfig()
	if err != nil {
		return
	}
	if cfg.Mail.Provider == "" {
		slog.Debug("mail through PocketBase's own mailer", "detail", "mail.provider is empty")
		return
	}
	if _, err := FromConfig(cfg.Mail); err != nil {
		slog.Error("mail cannot be sent", "err", err,
			"fix", "set "+cfg.Mail.APIKeyEnv+" (e.g. infisical run -- nabu serve), or fix mail.provider in nabu.yaml")
		return
	}
	slog.Info("mail through provider API", "provider", cfg.Mail.Provider, "from", cfg.Mail.SenderAddress)
}
