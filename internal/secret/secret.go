// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

// Package secret verschlüsselt kleine Geheimnisse wie Google-Tokens, bevor
// sie in die Datenbank kommen.
//
// Der Schlüssel liegt als eigene Datei (secret.key) im Datenordner und wird
// beim ersten Start zufällig erzeugt. Das schützt vor dem Fall, dass nur die
// Datenbank in falsche Hände gerät, etwa über eine Sicherungskopie oder
// einen geteilten Ordner. Wer den ganzen Datenordner hat, kann die Tokens
// lesen – deshalb gehört der Datenordner nicht in Cloud-Ordner, die andere
// einsehen können. Für den Serverbetrieb kann der Schlüssel später aus einer
// Umgebungsvariable oder einem Geheimnis-Speicher kommen.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const keySize = 32

// Box ver- und entschlüsselt mit AES-256-GCM.
type Box struct {
	aead cipher.AEAD
}

// New erzeugt eine Box aus einem 32 Byte langen Schlüssel.
func New(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("Schlüssel muss %d Byte lang sein", keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// LoadOrCreate liest den Schlüssel aus file oder legt ihn an.
func LoadOrCreate(file string) (*Box, error) {
	key, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, keySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return nil, err
		}
		// O_EXCL: Ein zweiter Start gleichzeitig überschreibt den Schlüssel nicht.
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return LoadOrCreate(file)
			}
			return nil, err
		}
		if _, err := f.Write(key); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, fmt.Errorf("Schlüsseldatei %s: %w", file, err)
	}
	return New(key)
}

// Seal verschlüsselt plain. additional bindet das Ergebnis an einen Zweck
// (z. B. "google-token:3"), damit sich Geheimtexte nicht vertauschen lassen.
func (b *Box) Seal(plain, additional []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, additional), nil
}

// Open entschlüsselt, was Seal erzeugt hat.
func (b *Box) Open(sealed, additional []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n+b.aead.Overhead() {
		return nil, errors.New("Geheimtext zu kurz")
	}
	plain, err := b.aead.Open(nil, sealed[:n], sealed[n:], additional)
	if err != nil {
		return nil, errors.New("Entschlüsseln fehlgeschlagen (falscher Schlüssel oder veränderte Daten)")
	}
	return plain, nil
}
