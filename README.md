# Anforderungen an den Industrial Package Manager (IPM)

## Primäre Anforderungen

### Rolle als Paketmanager:

Der IPM soll ein vollwertiger Paketmanager sein, der Abhängigkeiten verwaltet, Pakete installiert und Konflikte erkennt.

### Performance:

Schnelle Installation und Verwaltung von Paketen.

- Idee: Pakete in einem globalen Cache speichern + symbolische Links im Projekt verwenden, um Speicherplatz zu sparen; optional lokale Kopien der Pakete im Projekt ablegen.

### Sicherheit:

- Pakete müssen beim installieren (registry, cache) und auch danach verifizierbar sein (Signaturprüfung mit Private/Public Keys).
- Unterstützung eines Login-Mechanismus (z. B. Tokens) für alle npm kompatiblen Registries (gitlab, github, aws, azure), einschließlich Custom Registries

### Erweiterbarkeit:

- Installierte Pakete können den IPM um zusätzliche Kommandos erweitern (z. B. `./ipm compile` für ST/C# nach Installation eines Compiler-Pakets via `./ipm install`).

### OS-Agnostik:

- Kompatibilität mit Windows, Linux und macOS, inklusive konsolenagnostischer Ausführung (bash, cmd, PowerShell, etc.).

## Zusätzliche Anforderungen und Ideen

### Konsolenagnostische Skripte:

- Unterstützung für benutzerdefinierte Skripte (z. B. `./ipm run hello`) aus npm kompatiblen paketen, die unabhängig von der Shell funktionieren.

### Multi-Project-Workspace:

- Verwaltung mehrerer Projekte in einem Arbeitsbereich (ähnlich Monorepos), mit konsistenter Abhängigkeitsauflösung.

### OpenTelemetry-Unterstützung:

- Integration von Telemetrie (z. B. Tracing, Metrics) für Monitoring und Debugging, optional aktivierbar.

### Deterministisches Verhalten:

- Zwingende Anforderung im industriellen Umfeld: Der IPM muss bei gleichen Eingaben immer das gleiche Ergebnis liefern, um reproduzierbare Builds zu ermöglichen und es darf immer nur eine Version eines Pakets im Projektkontext existieren.
- Konsistente Erkennung und Meldung von Abhängigkeitskonflikten (z. B. wie bei `express@4.16.2`), mit hilfreichen Rückmeldungen an den Anwender über Problemursachen.

## Spezifische Details aus früheren Diskussionen

- Testfall: `express@4.16.2` wurde bewusst gewählt, da es unter der Bedingung "nur eine Paketversion pro Paket im Projektkontext" unlösbare Konflikte (z. B. `statuses`, `setprototypeof`) erzeugt – ideal zur Validierung des deterministischen Verhaltens und der Fehlerdiagnose.
- Benutzerhilfe: Rückmeldungen sollen nicht nur Konflikte melden, sondern auch Hinweise geben, welche Pakete betroffen sind und was der Anwender tun könnte.

## Nonfunctional Requirements

- Voll npm kompatibel
- Einfach zu installieren
- Einfach zu konfigurieren
- Einfach zu warten
- Leicht Erweiterbar
- Einfach zu bedienen
- Sicher
- Performant
- Plattformunabhängig
- Diagnosefähig
- Clean Code
- Testbar
- Dokumentiert
- Command Line Interface (CLI) Guidelines (https://clig.dev/?trk=public_post-text)

## Gemeinsames Verständnis

Das Ziel des IPM ist ein leistungsfähiger, sicherer und flexibler Paketmanager, der:

- Effizient arbeitet (Cache, Symlinks),
- Vertrauen schafft (Signaturen, Tokens),
- Anpassbar ist (Plugins, Skripte),
- Plattformunabhängig funktioniert,
- Diagnosefähig ist (deterministische Konflikterkennung mit klarer Rückmeldung).
