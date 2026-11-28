// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package google

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestLoadClient(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(body), 0o600)
		return p
	}
	good := write("ok.json", `{"installed":{"client_id":"x.apps.googleusercontent.com","client_secret":"s","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["http://localhost"]}}`)
	c, err := LoadClient(good)
	if err != nil || c.ID != "x.apps.googleusercontent.com" || c.TokenURL != "https://oauth2.googleapis.com/token" {
		t.Fatalf("%v %+v", err, c)
	}
	for name, body := range map[string]string{
		"leer.json":     `{}`,
		"kaputt.json":   `{`,
		"unsicher.json": `{"installed":{"client_id":"x","token_uri":"http://evil.example/token"}}`,
	} {
		if _, err := LoadClient(write(name, body)); err == nil {
			t.Errorf("%s akzeptiert", name)
		}
	}
}

func TestOAuthFlow(t *testing.T) {
	ctx := context.Background()
	f := newFakeGoogle(t)
	c := f.client()

	authURL, p := c.Begin("http://127.0.0.1:8470/")
	q := mustURL(t, authURL).Query()
	for _, k := range []string{"client_id", "redirect_uri", "state", "code_challenge", "scope"} {
		if q.Get(k) == "" {
			t.Errorf("%s fehlt", k)
		}
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("access_type") != "offline" {
		t.Error("PKCE oder offline fehlt")
	}
	if strings.Contains(authURL, p.Verifier) {
		t.Error("Verifier darf nicht in der Adresse stehen")
	}
	_, code := f.grant(authURL)

	// Falscher Verifier wird abgelehnt.
	bad := *p
	bad.Verifier = "falsch"
	if _, err := c.Exchange(ctx, f.srv.Client(), &bad, code); err == nil {
		t.Error("falscher Verifier akzeptiert")
	}
	_, code = f.grant(authURL)
	tok, err := c.Exchange(ctx, f.srv.Client(), p, code)
	if err != nil || tok.RefreshToken != "refresh-1" || !tok.Valid() {
		t.Fatalf("Exchange: %v %+v", err, tok)
	}
	email, err := c.Email(ctx, f.srv.Client(), tok)
	if err != nil || email != "andreas@example.com" {
		t.Fatalf("Email: %v %q", err, email)
	}
	nt, err := c.Refresh(ctx, f.srv.Client(), tok)
	if err != nil || nt.AccessToken == tok.AccessToken || nt.RefreshToken != "refresh-1" {
		t.Fatalf("Refresh: %v %+v", err, nt)
	}
	f.revoked = true
	if _, err := c.Refresh(ctx, f.srv.Client(), nt); !errors.Is(err, ErrRevoked) {
		t.Errorf("widerrufen: %v", err)
	}

	old := *p
	old.Created = time.Now().Add(-11 * time.Minute)
	if _, err := c.Exchange(ctx, f.srv.Client(), &old, code); err == nil {
		t.Error("alte Anmeldung akzeptiert")
	}
}
