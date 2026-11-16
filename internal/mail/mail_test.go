package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tools/mailer"

	"github.com/mrmxf/nabu/embedfs"
)

// fake records the last request a provider made.
type fake struct {
	header http.Header
	body   map[string]any
	status int
}

func (f *fake) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.header = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		f.body = map[string]any{}
		_ = json.Unmarshal(raw, &f.body)
		if f.status == 0 {
			f.status = http.StatusCreated
		}
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"message":"bad things"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func msg() *mailer.Message {
	return &mailer.Message{
		From:    mail.Address{Name: "PocketBase", Address: "support@nabu.example"},
		To:      []mail.Address{{Address: "alice@nabu.example"}},
		Subject: "Your sign-in code",
		HTML:    "<p>123456</p>",
	}
}

func TestBrevo(t *testing.T) {
	f := &fake{}
	srv := f.server(t)
	s, err := New(Config{Provider: "brevo", APIKey: "k-brevo", Endpoint: srv.URL,
		From: mail.Address{Name: "Nabu", Address: "no-reply@nabu.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatal(err)
	}
	if got := f.header.Get("api-key"); got != "k-brevo" {
		t.Errorf("api-key header = %q", got)
	}
	sender := f.body["sender"].(map[string]any)
	if sender["email"] != "no-reply@nabu.example" || sender["name"] != "Nabu" {
		t.Errorf("sender = %v: the configured From must replace PocketBase's", sender)
	}
	to := f.body["to"].([]any)[0].(map[string]any)
	if to["email"] != "alice@nabu.example" || f.body["subject"] != "Your sign-in code" || f.body["htmlContent"] != "<p>123456</p>" {
		t.Errorf("body = %v", f.body)
	}
}

func TestMailtrap(t *testing.T) {
	f := &fake{status: http.StatusOK}
	srv := f.server(t)
	s, err := New(Config{Provider: "Mailtrap", APIKey: "k-trap", Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatal(err)
	}
	if got := f.header.Get("Authorization"); got != "Bearer k-trap" {
		t.Errorf("Authorization = %q", got)
	}
	from := f.body["from"].(map[string]any)
	if from["email"] != "support@nabu.example" || f.body["html"] != "<p>123456</p>" {
		t.Errorf("with no configured From the message's own is kept: %v", f.body)
	}
}

func TestSendErrors(t *testing.T) {
	f := &fake{status: http.StatusUnauthorized}
	srv := f.server(t)
	s, _ := New(Config{Provider: "brevo", APIKey: "k", Endpoint: srv.URL})
	err := s.Send(context.Background(), msg())
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "k\"") {
		t.Fatalf("err = %v: want the status, never the key", err)
	}
	if err := s.Send(context.Background(), &mailer.Message{Subject: "x"}); err != ErrNoRecipient {
		t.Errorf("no recipient: err = %v", err)
	}
	if _, err := New(Config{Provider: "pigeon", APIKey: "k"}); err == nil {
		t.Error("an unknown provider must fail")
	}
	if _, err := New(Config{Provider: "brevo"}); err == nil {
		t.Error("a missing key must fail")
	}
}

func TestFromConfig(t *testing.T) {
	if s, err := FromConfig(embedfs.MailConfig{}); s != nil || err != nil {
		t.Errorf("empty provider = (%v, %v), want PocketBase's mailer (nil, nil)", s, err)
	}
	t.Setenv("NABU_TEST_MAIL_KEY", "")
	cfg := embedfs.MailConfig{Provider: "brevo", APIKeyEnv: "NABU_TEST_MAIL_KEY"}
	if _, err := FromConfig(cfg); err == nil || !strings.Contains(err.Error(), "NABU_TEST_MAIL_KEY") {
		t.Errorf("unset key: err = %v, want it to name the env var", err)
	}
	t.Setenv("NABU_TEST_MAIL_KEY", "k")
	if s, err := FromConfig(cfg); err != nil || s.Name() != "brevo" {
		t.Errorf("set key = (%v, %v)", s, err)
	}
	if got := Providers(); strings.Join(got, ",") != "brevo,mailtrap" {
		t.Errorf("Providers() = %v", got)
	}
}

func TestSenderAddress(t *testing.T) {
	cfg := embedfs.MailConfig{SenderAddress: "no-reply@nabu.example", SenderAddressEnv: "NABU_TEST_SENDER"}
	t.Setenv("NABU_TEST_SENDER", "")
	if got := SenderAddress(cfg); got != "no-reply@nabu.example" {
		t.Errorf("unset env: %q, want sender_address", got)
	}
	t.Setenv("NABU_TEST_SENDER", "course@nabu.example")
	if got := SenderAddress(cfg); got != "course@nabu.example" {
		t.Errorf("set env: %q, want its value", got)
	}
	cfg.SenderAddressEnv = ""
	if got := SenderAddress(cfg); got != "no-reply@nabu.example" {
		t.Errorf("no env named: %q", got)
	}
}
