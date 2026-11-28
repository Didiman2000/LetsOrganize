// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package secret

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSealOpen(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secret.key")
	b, err := LoadOrCreate(file)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal([]byte("refresh-token"), []byte("google-token:1"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("refresh-token")) {
		t.Error("Klartext im Geheimtext")
	}
	// Mit demselben Schlüssel (neu geladen) wieder lesbar.
	b2, err := LoadOrCreate(file)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := b2.Open(sealed, []byte("google-token:1"))
	if err != nil || string(plain) != "refresh-token" {
		t.Fatalf("Open: %v %q", err, plain)
	}
	if _, err := b2.Open(sealed, []byte("google-token:2")); err == nil {
		t.Error("anderer Zweck akzeptiert")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := b2.Open(sealed, []byte("google-token:1")); err == nil {
		t.Error("veränderter Geheimtext akzeptiert")
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(file); st.Mode().Perm() != 0o600 {
			t.Errorf("Rechte der Schlüsseldatei: %v", st.Mode().Perm())
		}
	}
	other, _ := LoadOrCreate(filepath.Join(t.TempDir(), "other.key"))
	good, _ := b.Seal([]byte("x"), nil)
	if _, err := other.Open(good, nil); err == nil {
		t.Error("fremder Schlüssel akzeptiert")
	}
}
