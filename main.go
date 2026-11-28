// LetsOrganize – persönlicher Organizer im Ringbuch-Stil
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version. See LICENSE.

package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	// Zeitzonen-Datenbank einbetten: Windows bringt keine mit, ohne sie
	// ließen sich Serien nicht in Europe/Berlin usw. berechnen.
	_ "time/tzdata"

	"github.com/Didiman2000/LetsOrganize/internal/google"
	"github.com/Didiman2000/LetsOrganize/internal/secret"
	"github.com/Didiman2000/LetsOrganize/internal/server"
	"github.com/Didiman2000/LetsOrganize/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8470", "Adresse, auf der LetsOrganize lauscht")
	dataDir := flag.String("data", defaultDataDir(), "Ordner für Daten und eigene Themen")
	flag.Parse()

	if err := os.MkdirAll(filepath.Join(*dataDir, "themes"), 0o755); err != nil {
		log.Fatalf("Datenordner %s kann nicht angelegt werden: %v", *dataDir, err)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(*dataDir, "letsorganize.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	version, _ := db.Version(ctx)

	box, err := secret.LoadOrCreate(filepath.Join(*dataDir, "secret.key"))
	if err != nil {
		log.Fatal(err)
	}
	var gclient *google.Client
	clientFile := filepath.Join(*dataDir, "google-client.json")
	if _, err := os.Stat(clientFile); err == nil {
		if gclient, err = google.LoadClient(clientFile); err != nil {
			log.Printf("Google-Abgleich aus: %v", err)
		}
	} else {
		log.Printf("Google-Abgleich aus: %s fehlt (siehe docs/GOOGLE-EINRICHTUNG.md)", clientFile)
	}
	gm := google.NewManager(db, box, gclient)
	local, err := db.EnsureLocalUser(ctx)
	if err != nil {
		log.Fatal(err)
	}
	go gm.Run(ctx, func() []int64 { return []int64{local.ID} }, 5*time.Minute)

	handler, err := server.New(server.Config{
		Assets:  assets,
		DataDir: *dataDir,
		Store:   db,
		Google:  gm,
	})
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("LetsOrganize läuft auf http://%s (Daten: %s, Schema %d)", *addr, *dataDir, version)
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
