// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

// Package google spricht mit Google Kalender, Kontakte und Aufgaben.
//
// Es verwendet nur die Go-Standardbibliothek: OAuth 2.0 mit PKCE für
// Desktop-Apps und die REST-Schnittstellen von Google. Alle Adressen sind
// einstellbar, damit die Tests gegen einen nachgebildeten Server laufen.
package google

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Scopes sind die Berechtigungen, die LetsOrganize anfragt.
var Scopes = []string{
	"openid",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/calendar",
	"https://www.googleapis.com/auth/contacts",
	"https://www.googleapis.com/auth/tasks",
}

// Client beschreibt den OAuth-Zugang aus google-client.json.
type Client struct {
	ID          string
	Secret      string
	AuthURL     string
	TokenURL    string
	UserinfoURL string
}

// LoadClient liest die von Google heruntergeladene JSON-Datei
// (Typ „Desktop-App“ unter "installed", zur Not auch "web").
func LoadClient(file string) (*Client, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var f map[string]struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		AuthURI      string `json:"auth_uri"`
		TokenURI     string `json:"token_uri"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s ist keine gültige Google-Client-Datei: %w", file, err)
	}
	entry, ok := f["installed"]
	if !ok {
		entry, ok = f["web"]
	}
	if !ok || entry.ClientID == "" {
		return nil, fmt.Errorf("%s enthält keinen Client (installed oder web)", file)
	}
	c := &Client{
		ID: entry.ClientID, Secret: entry.ClientSecret,
		AuthURL: entry.AuthURI, TokenURL: entry.TokenURI,
		UserinfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
	}
	if c.AuthURL == "" {
		c.AuthURL = "https://accounts.google.com/o/oauth2/auth"
	}
	if c.TokenURL == "" {
		c.TokenURL = "https://oauth2.googleapis.com/token"
	}
	for _, u := range []string{c.AuthURL, c.TokenURL} {
		if p, err := url.Parse(u); err != nil || p.Scheme != "https" {
			return nil, fmt.Errorf("%s: unsichere Adresse %q", file, u)
		}
	}
	return c, nil
}

// Token ist ein Zugangstoken mit Erneuerungstoken.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	Scope        string    `json:"scope"`
}

// Valid meldet, ob das Zugangstoken noch mindestens eine Minute gilt.
func (t *Token) Valid() bool {
	return t.AccessToken != "" && time.Until(t.Expiry) > time.Minute
}

// Pending ist eine angefangene Anmeldung (zwischen Weiterleitung und Rückkehr).
type Pending struct {
	State    string
	Verifier string
	Redirect string
	Created  time.Time
}

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand fällt praktisch nie aus
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Begin startet eine Anmeldung und liefert die Adresse, zu der der Browser
// geschickt wird. redirect ist die lokale Rücksprungadresse.
func (c *Client) Begin(redirect string) (string, *Pending) {
	p := &Pending{State: randomString(24), Verifier: randomString(48), Redirect: redirect, Created: time.Now()}
	sum := sha256.Sum256([]byte(p.Verifier))
	q := url.Values{
		"client_id":             {c.ID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"scope":                 {strings.Join(Scopes, " ")},
		"state":                 {p.State},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"access_type":           {"offline"},
		"prompt":                {"consent"}, // sonst gibt Google beim zweiten Mal kein Erneuerungstoken
	}
	return c.AuthURL + "?" + q.Encode(), p
}

// tokenResponse ist die Antwort des Token-Endpunkts.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// ErrRevoked meldet, dass Google den Zugang nicht mehr akzeptiert
// (widerrufen, abgelaufen oder Testmodus nach sieben Tagen).
var ErrRevoked = errors.New("Google-Zugang abgelaufen oder widerrufen – bitte neu verbinden")

func (c *Client) token(ctx context.Context, hc *http.Client, form url.Values) (*Token, error) {
	form.Set("client_id", c.ID)
	if c.Secret != "" {
		form.Set("client_secret", c.Secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google nicht erreichbar: %w", err)
	}
	defer res.Body.Close()
	var tr tokenResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tr); err != nil {
		return nil, fmt.Errorf("Antwort von Google unlesbar (Status %d)", res.StatusCode)
	}
	if tr.Error == "invalid_grant" {
		return nil, ErrRevoked
	}
	if res.StatusCode != http.StatusOK || tr.AccessToken == "" {
		return nil, fmt.Errorf("Google lehnt ab: %s %s", tr.Error, tr.ErrorDesc)
	}
	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		Scope:        tr.Scope,
	}, nil
}

// Exchange tauscht den Code aus der Rückleitung gegen Tokens.
func (c *Client) Exchange(ctx context.Context, hc *http.Client, p *Pending, code string) (*Token, error) {
	if time.Since(p.Created) > 10*time.Minute {
		return nil, errors.New("Anmeldung zu lange her – bitte neu starten")
	}
	t, err := c.token(ctx, hc, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.Redirect},
		"code_verifier": {p.Verifier},
	})
	if err != nil {
		return nil, err
	}
	if t.RefreshToken == "" {
		return nil, errors.New("Google hat kein Erneuerungstoken geliefert – bitte den Zugang in deinem Google-Konto entfernen und neu verbinden")
	}
	return t, nil
}

// Refresh erneuert ein abgelaufenes Zugangstoken.
func (c *Client) Refresh(ctx context.Context, hc *http.Client, t *Token) (*Token, error) {
	if t.RefreshToken == "" {
		return nil, ErrRevoked
	}
	nt, err := c.token(ctx, hc, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {t.RefreshToken},
	})
	if err != nil {
		return nil, err
	}
	if nt.RefreshToken == "" {
		nt.RefreshToken = t.RefreshToken
	}
	return nt, nil
}

// Email fragt die E-Mail-Adresse des angemeldeten Kontos ab.
func (c *Client) Email(ctx context.Context, hc *http.Client, t *Token) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.UserinfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	res, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var info struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Kontoinformation: Status %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&info); err != nil {
		return "", err
	}
	if info.Email == "" {
		return "", errors.New("Google liefert keine E-Mail-Adresse")
	}
	return info.Email, nil
}
