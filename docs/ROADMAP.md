# LetsOrganize – Roadmap Version 1

Stand: 27.11.2026

## Ziel

Persönlicher Organizer im Browser mit der Optik eines hochwertigen Leder-Ringbuchs mit farbigen Registern, angelehnt an das klassische Organizer-Layout, aber mit Abgleich zu Google Kalender, Google Kontakte und Google Tasks. Zunächst für den eigenen Gebrauch, als Open Source unter der GPL 3.0.

## Entscheidungen

- Browser-Anwendung, Backend in Go: eine Programmdatei, lokal startbar, später im Container betreibbar
- Eigenes Design ohne Original-Grafiken und ohne den Namen „Lotus“
- Oberfläche vollständig über **Themen** gestaltbar (Farben, Einband, Papier je Einlage, Schriften, Registerformen); Themen als Datei austauschbar
- **Mehrsprachig** von Anfang an: alle Texte in Sprachdateien, Start mit Deutsch und Englisch, Datums- und Zahlenformate je Sprache; Beschriftungen kommen aus der Sprachdatei, nicht aus dem Thema
- **Rechts-nach-links-Sprachen** (z. B. Arabisch, Hebräisch): Layout nur mit logischen CSS-Eigenschaften, Ringbuch gespiegelt
- Google-Zugang über eigenes OAuth-Projekt; bei Open Source legt jeder Nutzer seinen eigenen Zugang an
- Import der Altdaten aus Organizer 6/6.1 per CSV-/Text-Export, kein Einlesen des Binärformats
- Vorbereitung auf Serverbetrieb: Anmeldung, Daten je Nutzer getrennt, Google-Tokens verschlüsselt
- Lizenz: GPL 3.0 oder später

## Register in Version 1

1. Kalender – Tages-, Wochen- (Doppelseite) und Monatsansicht
2. Aufgaben – Abgleich mit Google Tasks
3. Adressen – Abgleich mit Google Kontakte, Register A–Z
4. Notizen – lokal gespeichert
5. Planer – Jahresübersicht mit farbigen Markierungen
6. Jahrestage – automatisch aus Geburtstagen der Kontakte, plus eigene

MCP-Schnittstelle für externe KI-Agenten (siehe Phase 7). Später: Anrufe, Gmail-Anbindung, Verknüpfungen zwischen Einträgen, Mehrbenutzer-Server.

## Phasen

### Phase 0 – Vorbereitung
- [x] Name (LetsOrganize), Repository, Lizenz (GPL 3.0)
- [ ] Alte Organizer-Installation starten (ggf. in einer VM)
- [ ] Probeexport aller Bereiche als CSV/Text erstellen und Felder dokumentieren
- [ ] Google-Cloud-Projekt anlegen, Calendar-, People- und Tasks-API aktivieren, OAuth-Client erstellen – Anleitung: [GOOGLE-EINRICHTUNG.md](GOOGLE-EINRICHTUNG.md)

### Phase 1 – Grundgerüst
- [x] Go-Projekt mit Webserver, eingebettete Oberfläche, Sicherheits-Header (CSP)
- [x] Mehrsprachigkeit: Sprachdateien (de, en, ar), Sprachwahl, Wochenbeginn je Sprache, Rückfall auf Englisch, Test auf Vollständigkeit
- [x] Rechts-nach-links: logische CSS-Eigenschaften, gespiegeltes Ringbuch, Test mit Arabisch
- [x] Themen: zwei mitgelieferte Themen, eigene Themen im Datenordner, strenge Prüfung der Werte
- [x] Ringbuch-Oberfläche: Einband, Ringe, Register, Wochenansicht, Jahresplaner (ohne Daten)
- [x] SQLite-Datenbank mit Migrationen (Treiber `modernc.org/sqlite`, reines Go)
- [x] Datenmodell: Termine, Aufgaben, Kontakte, Notizen, Jahrestage, Planer-Einträge, Kategorien, Google-Konten, Abgleich-Status
- [x] Nutzer-Trennung im Datenmodell, Benutzer und Sitzungen (PBKDF2-Passwörter, gehashte Sitzungstokens), lokaler Benutzer
- [ ] Anmeldeseite für den Serverbetrieb (lokal ohne Anmeldung)
- [ ] Build für Windows mit Programmsymbol, Dockerfile

### Phase 2 – Ringbuch-Oberfläche ausbauen
- [ ] Blätter-Animation verfeinern (abschaltbar)
- [ ] Themen anlegen, ändern, importieren und exportieren in der Oberfläche; Reihenfolge der Abschnitte einstellbar
- [ ] Hintergrundbilder für Einband und Papier im Thema (geprüft, nur aus dem Themenpaket)
- [ ] Tastaturbedienung und Druckansicht

### Phase 3 – Register ohne Google
- [x] Kalender: Termine anlegen, ändern, löschen (Wochenansicht, ganztägig, Schnittstelle /api/events mit CSRF-Schutz)
- [x] Kalender: wiederkehrende Termine (RRULE: täglich, wöchentlich mit Wochentagen, alle zwei Wochen, monatlich, jährlich; Ende nach Anzahl oder Datum), richtige Uhrzeit über die Zeitumstellung, einzelne Vorkommen löschen (Migration 0003)
- [ ] Kalender: Erinnerungen, Verschieben per Ziehen, Monatsregeln wie „jeden 2. Dienstag“
- [x] Aufgaben: anlegen per Schnell-Eingabe, abhaken, Fälligkeit, Priorität, Notiz, erledigte Liste (Schnittstelle /api/tasks)
- [ ] Aufgaben: Reihenfolge per Ziehen, Kategorien
- [x] Adressen: Register je Sprache (A–Z, arabisches Alphabet), Suche in Namen, Firma und Feldern, Telefon/E-Mail/Anschrift/Webseite, Geburtstag mit oder ohne Jahr (Schnittstelle /api/contacts)
- [ ] Adressen: Fotos, Gruppen, Export als vCard
- [x] Notizen: Liste und Seite mit automatischem Speichern (/api/notes)
- [x] Planer: Jahresübersicht mit farbigen Zeiträumen, Jahr blättern (/api/planner, Migration 0002 für Farben)
- [x] Jahrestage: eigene Jahrestage plus Geburtstage aus dem Adressbuch, Alter, Sprung zum Kontakt (/api/anniversaries)
- [ ] Erinnerungen, Kategorien

### Phase 4 – Google-Abgleich
- [x] OAuth-Anmeldung (Desktop-Client, PKCE, Rückleitung auf 127.0.0.1), Token mit AES-256-GCM verschlüsselt (Schlüssel secret.key im Datenordner), automatische Erneuerung, Trennen mit Widerruf
- [x] Kalender: Abgleich des Hauptkalenders in beide Richtungen alle 5 Minuten und per Knopf (Sync-Token, ETags, Serien mit RRULE/EXDATE, verschobene und abgesagte Einzeltermine, Migration 0004)
- [ ] Kontakte: People API, Geburtstage → Jahrestage
- [ ] Aufgaben: Google Tasks
- [x] Konfliktregel: jüngere Änderung gewinnt; bei Google gelöscht, lokal geändert → neu anlegen
- [x] Offline-Betrieb: lokale Änderungen werden vorgemerkt und beim nächsten Abgleich übertragen
- [x] Mehrere Kalender: Kalenderliste von Google, Auswahl je Kalender, Farben, Kalenderwahl beim Anlegen, nur lesbare Kalender (Migration 0005)
- [ ] Termin in einen anderen Kalender verschieben

### Phase 5 – Import der Altdaten
- [ ] CSV-Import je Bereich mit Feldzuordnung und Vorschau
- [ ] Zeichensatz prüfen (Umlaute, vermutlich Windows-1252)
- [ ] Wiederkehrende Termine und Jahrestage soweit möglich erkennen
- [ ] Option: importierte Daten direkt zu Google übertragen

### Phase 6 – Abschluss Version 1
- [ ] Datensicherung und Export (ICS, vCard, CSV)
- [ ] Tests im Alltag, Fehler beheben
- [ ] Anleitung für den eigenen Google-Zugang
- [ ] Erstes Release auf GitHub

### Phase 7 – MCP-Schnittstelle für KI-Agenten
- [ ] MCP-Server im Programm (Streamable HTTP), lokal und im Container nutzbar
- [ ] Werkzeuge zum Lesen: Termine im Zeitraum, offene Aufgaben, Kontakte suchen, Notizen, Jahrestage, Planer
- [ ] Werkzeuge zum Schreiben: Termine und Aufgaben anlegen, ändern, erledigen, löschen; Notizen anlegen
- [ ] Zugriff nur mit eigenem Token je Agent, Rechte getrennt nach Lesen und Schreiben sowie nach Bereich
- [ ] Protokoll aller Änderungen durch Agenten; Änderungen laufen über denselben Google-Abgleich
- [ ] Anleitung zur Einbindung in Claude und andere MCP-Clients

## Offene Fragen

- Welche Kalenderansichten sind am wichtigsten (Woche als Doppelseite wie im Original?)
- Inhalt des CSV-Exports aus Organizer 6
- Weitere Sprachen außer Deutsch, Englisch und Arabisch
