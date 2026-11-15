package mail

import (
	"context"
	"net/mail"

	"github.com/pocketbase/pocketbase/tools/mailer"
)

// person is the {email, name} shape both providers use.
type person struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

func people(as []mail.Address) []person {
	out := make([]person, 0, len(as))
	for _, a := range as {
		if a.Address != "" {
			out = append(out, person{Email: a.Address, Name: a.Name})
		}
	}
	return out
}

// ---- Brevo: POST https://api.brevo.com/v3/smtp/email, header api-key ----

// BrevoEndpoint is Brevo's transactional email API.
const BrevoEndpoint = "https://api.brevo.com/v3/smtp/email"

type brevo struct{ c Config }

func newBrevo(c Config) Sender {
	if c.Endpoint == "" {
		c.Endpoint = BrevoEndpoint
	}
	return &brevo{c}
}

func (b *brevo) Name() string { return "brevo" }

type brevoBody struct {
	Sender      person            `json:"sender"`
	To          []person          `json:"to"`
	Cc          []person          `json:"cc,omitempty"`
	Bcc         []person          `json:"bcc,omitempty"`
	Subject     string            `json:"subject"`
	HTMLContent string            `json:"htmlContent,omitempty"`
	TextContent string            `json:"textContent,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

func (b *brevo) Send(ctx context.Context, m *mailer.Message) error {
	to := people(m.To)
	if len(to) == 0 {
		return ErrNoRecipient
	}
	from := b.c.from(m)
	return postJSON(ctx, b.c.Client, b.c.Endpoint, map[string]string{"api-key": b.c.APIKey}, brevoBody{
		Sender:      person{Email: from.Address, Name: from.Name},
		To:          to,
		Cc:          people(m.Cc),
		Bcc:         people(m.Bcc),
		Subject:     m.Subject,
		HTMLContent: m.HTML,
		TextContent: m.Text,
		Headers:     m.Headers,
	})
}

// ---- Mailtrap: POST https://send.api.mailtrap.io/api/send, bearer token ----

// MailtrapEndpoint is Mailtrap's sending API.
const MailtrapEndpoint = "https://send.api.mailtrap.io/api/send"

type mailtrap struct{ c Config }

func newMailtrap(c Config) Sender {
	if c.Endpoint == "" {
		c.Endpoint = MailtrapEndpoint
	}
	return &mailtrap{c}
}

func (t *mailtrap) Name() string { return "mailtrap" }

type mailtrapBody struct {
	From    person            `json:"from"`
	To      []person          `json:"to"`
	Cc      []person          `json:"cc,omitempty"`
	Bcc     []person          `json:"bcc,omitempty"`
	Subject string            `json:"subject"`
	HTML    string            `json:"html,omitempty"`
	Text    string            `json:"text,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (t *mailtrap) Send(ctx context.Context, m *mailer.Message) error {
	to := people(m.To)
	if len(to) == 0 {
		return ErrNoRecipient
	}
	from := t.c.from(m)
	return postJSON(ctx, t.c.Client, t.c.Endpoint, map[string]string{"Authorization": "Bearer " + t.c.APIKey}, mailtrapBody{
		From:    person{Email: from.Address, Name: from.Name},
		To:      to,
		Cc:      people(m.Cc),
		Bcc:     people(m.Bcc),
		Subject: m.Subject,
		HTML:    m.HTML,
		Text:    m.Text,
		Headers: m.Headers,
	})
}
