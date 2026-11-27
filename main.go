// LetsOrganize – persönlicher Organizer im Ringbuch-Stil
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version. See LICENSE.

package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Didiman2000/LetsOrganize/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8470", "Adresse, auf der LetsOrganize lauscht")
	dataDir := flag.String("data", defaultDataDir(), "Ordner für Daten und eigene Themen")
	flag.Parse()

	if err := os.MkdirAll(filepath.Join(*dataDir, "themes"), 0o755); err != nil {
		log.Fatalf("Datenordner %s kann nicht angelegt werden: %v", *dataDir, err)
	}

	handler, err := server.New(server.Config{
		Assets:  assets,
		DataDir: *dataDir,
	})
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("LetsOrganize läuft auf http://%s (Daten: %s)", *addr, *dataDir)
	log.Fatal(srv.ListenAndServe())
}

// defaultDataDir liefert den üblichen Ordner für Programmdaten des Benutzers,
// z. B. %AppData%\LetsOrganize unter Windows oder ~/.config/LetsOrganize unter Linux.
func defaultDataDir() string {
	if env := os.Getenv("LETSORGANIZE_DATA"); env != "" {
		return env
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "LetsOrganize")
	}
	return "LetsOrganize-Daten"
}
