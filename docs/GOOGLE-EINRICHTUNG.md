# Google-Zugang für LetsOrganize einrichten

Damit LetsOrganize Termine, Kontakte und Aufgaben mit Google abgleichen kann, braucht jede Installation einen eigenen Zugang bei Google. Das ist einmalige Arbeit von etwa 15 Minuten. Deine Daten laufen dabei direkt zwischen deinem Rechner und Google – niemand sonst ist beteiligt.

Stand: November 2026. Google ändert die Oberfläche der Cloud Console gelegentlich; die Bezeichnungen können leicht abweichen.

## Überblick

1. Google-Cloud-Projekt anlegen
2. Drei Schnittstellen freischalten: Kalender, Kontakte (People), Aufgaben (Tasks)
3. Google Auth Platform einrichten: Name, Zielgruppe, Berechtigungen
4. Zugang (OAuth-Client) vom Typ „Desktop-App“ erstellen und die JSON-Datei herunterladen
5. Datei in den LetsOrganize-Datenordner legen

## 1. Projekt anlegen

1. <https://console.cloud.google.com/> mit dem Google-Konto öffnen, dessen Kalender und Kontakte abgeglichen werden sollen.
2. Oben links auf die Projektauswahl klicken → **Neues Projekt**.
3. Name: `LetsOrganize` (frei wählbar). Organisation: keine bzw. die vorgeschlagene. **Erstellen**.
4. Sicherstellen, dass oben das neue Projekt ausgewählt ist.

Ein Rechnungskonto ist nicht nötig; die drei Schnittstellen sind für diesen Zweck kostenlos.

## 2. Schnittstellen freischalten

Unter **APIs & Dienste → Bibliothek** nacheinander suchen und jeweils **Aktivieren** klicken:

| Schnittstelle | Wofür |
| --- | --- |
| Google Calendar API | Termine |
| People API | Kontakte und Geburtstage |
| Google Tasks API | Aufgaben |

## 3. Google Auth Platform einrichten

Im Menü **Google Auth Platform** (früher „OAuth-Zustimmungsbildschirm“) auf **Jetzt starten** bzw. **Get started** klicken.

**Branding (App-Informationen)**
- App-Name: `LetsOrganize`
- Nutzer-Support-E-Mail: deine Adresse
- Kontakt für Entwickler: deine Adresse

**Zielgruppe (Audience)**
- Bei einem normalen Gmail-Konto ist nur **Extern** möglich. Bei Google Workspace kannst du **Intern** wählen – dann entfallen die Hinweise unten.
- Unter **Testnutzer** dein eigenes Google-Konto eintragen.

**Datenzugriff (Data Access)** → **Bereiche hinzufügen** und diese vier eintragen (über „Manuell hinzufügen“ einfügen):

```
https://www.googleapis.com/auth/calendar
https://www.googleapis.com/auth/contacts
https://www.googleapis.com/auth/tasks
https://www.googleapis.com/auth/userinfo.email
```

Kalender, Kontakte und Aufgaben gelten bei Google als „sensible“ Bereiche. Für den eigenen Gebrauch ist das kein Problem, siehe Abschnitt 6.

## 4. Zugang erstellen

1. **Google Auth Platform → Clients** → **Client erstellen**.
2. Anwendungstyp: **Desktop-App**. Name: `LetsOrganize auf <Rechnername>`.
3. **Erstellen**, dann **JSON herunterladen**.

Typ „Desktop-App“, weil LetsOrganize lokal läuft und die Anmeldung über die lokale Adresse `http://127.0.0.1:<Port>` zurückkommt. Für den späteren Betrieb als Server im Container kommt ein zweiter Client vom Typ „Webanwendung“ dazu – das beschreiben wir, wenn es so weit ist.

## 5. Datei ablegen

Die heruntergeladene Datei heißt etwa `client_secret_1234…apps.googleusercontent.com.json`. Umbenennen in `google-client.json` und in den Datenordner von LetsOrganize legen:

| System | Ordner |
| --- | --- |
| Windows | `%AppData%\LetsOrganize\` |
| Linux | `~/.config/LetsOrganize/` |
| macOS | `~/Library/Application Support/LetsOrganize/` |

Wer LetsOrganize mit `-data <Ordner>` oder `LETSORGANIZE_DATA` startet, legt die Datei dort ab.

**Wichtig:** Die Datei enthält das Client-Geheimnis deines Projekts. Sie gehört nicht ins Git-Repository und nicht in geteilte Ordner. Die `.gitignore` von LetsOrganize schließt `google-client*.json` und `client_secret*.json` aus.

## 6. Testmodus oder veröffentlichen?

Solange die App bei Google im Status **Testen** steht, gilt eine Einschränkung: Die Anmeldung von Testnutzern läuft **nach sieben Tagen ab**, sobald Bereiche wie Kalender oder Kontakte im Spiel sind. Du müsstest dich dann jede Woche neu bei Google anmelden.

Zwei Wege:

- **Im Testmodus bleiben** – sicherste Variante, aber wöchentlich neu anmelden.
- **Veröffentlichen** (empfohlen für den Dauerbetrieb): unter **Zielgruppe** auf **App veröffentlichen** klicken. Eine Prüfung durch Google ist für den eigenen Gebrauch nicht nötig. Bei der Anmeldung zeigt Google dann den Hinweis „Google hat diese App nicht überprüft“. Da es deine eigene App ist, kannst du über **Erweitert → Weiter zu LetsOrganize (unsicher)** fortfahren. Ohne Prüfung können sich höchstens 100 Konten anmelden – für den privaten Gebrauch mehr als genug.

Wer LetsOrganize später für andere anbietet, braucht ein eigenes, von Google geprüftes Projekt. Bei der Open-Source-Weitergabe richtet sich jeder Nutzer nach dieser Anleitung seinen eigenen Zugang ein.

## 7. Prüfen

Wenn alles erledigt ist, sollte gelten:

- [ ] Projekt `LetsOrganize` existiert und ist ausgewählt
- [ ] Calendar API, People API und Tasks API sind aktiviert
- [ ] Die vier Bereiche sind unter „Datenzugriff“ eingetragen
- [ ] Dein Konto steht unter „Testnutzer“ (oder die App ist veröffentlicht)
- [ ] `google-client.json` liegt im Datenordner

Den Inhalt der Datei bitte **nicht** in Chats, Tickets oder E-Mails kopieren.

## Quellen

- [Google Cloud Help: Manage App Audience](https://support.google.com/cloud/answer/15549945) – Testnutzer, Sieben-Tage-Ablauf, Veröffentlichen, Grenze von 100 Nutzern
