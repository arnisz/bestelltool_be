# Deployment-Strategie für Datenbankmigrationen

Dieses Dokument definiert verbindlich, wie eingebettete Migrationen (`RUN_MIGRATIONS`) in unterschiedlichen Umgebungen betrieben werden.

## 1. Grundsatz

- Der Server führt ausschließlich **Up-Migrationen** (`*.up.sql`) aus.
- Der integrierte Runner ist für kontrollierte Schema-Weiterentwicklung vorgesehen, nicht für automatisierte Rollbacks.

## 2. Lokale Entwicklung & Testing

- Für lokale Entwicklung, manuelle Tests und E2E-Szenarien ist `RUN_MIGRATIONS=true` (oder `1`) ausdrücklich empfohlen.
- Ziel: schneller, reproduzierbarer Start ohne separaten Migrationsschritt.
- Voraussetzung: typischerweise Single-Instance-Betrieb (kein paralleles Hochfahren mehrerer Replikas gegen dieselbe DB).

## 3. Staging/Produktion (Multi-Instanz)

- In Staging/Produktion mit mehreren parallel startenden App-Instanzen gilt: `RUN_MIGRATIONS=false`.
- Begründung: parallele Migrationen beim Replica-Start erhöhen das Risiko von Start-Races, Lock-Wartezeiten und unnötiger Kopplung von Schema-Änderung und App-Rollout.
- Migrationen werden in diesen Umgebungen **vor** dem App-Rollout über einen dedizierten Mechanismus ausgeführt, z. B.:
  - separater CI/CD-Release-Job,
  - Kubernetes Init-Container / Job (einmalig pro Release),
  - dedizierter One-shot-Migration-Task.
- Reihenfolge für Deployments:
  1. Migration-Task ausführen und erfolgreich abschließen.
  2. Ergebnis verifizieren (z. B. `schema_migrations`, Health-Checks/Smoke-Checks).
  3. Erst danach neue App-Container/Replikas ausrollen.

## 4. Rollback-Policy

- Das Server-Binary darf in keiner Umgebung automatisiert `.down.sql` ausführen.
- Down-Migrationen sind potenziell destruktiv und erfordern einen **manuellen, administrativen Eingriff** (z. B. CLI-Tool oder manuelles SQL nach Freigabeprozess).
- Standardstrategie bei fehlgeschlagenen Deployments ist bevorzugt ein **Forward-Fix** (korrigierende Up-Migration + erneutes Deployment), statt automatisierter Down-Rollbacks.

## 5. Betriebsregeln (verbindlich)

- Lokal/Test: `RUN_MIGRATIONS=true` erlaubt.
- Staging/Produktion: `RUN_MIGRATIONS=false` verpflichtend, Migrationen via dediziertem Pre-Deployment-Schritt.
- Keine automatische Ausführung von `.down.sql` durch Anwendungscode.

## 6. Docker-Compose-Ableitung (Dev vs. Staging/Prod)

- `compose.yml` im Repository ist **explizit ein Dev-Compose**:
  - Backend läuft mit `APP_ENV=dev` und `RUN_MIGRATIONS=true`.
  - `AUTH_STATIC_TOKENS` ist nur hier zulässig (SEC-26).
  - Service `db` ist persistent (named volume) für lokale Entwicklung.
  - Service `db-test` (Profile `test`) ist flüchtig (tmpfs) für reproduzierbare, leere Testläufe.
- Für Staging/Produktion wird **keine** 1:1-Nutzung dieser Compose-Datei empfohlen:
  - Migrationen laufen vorher in einem dedizierten Schritt (Abschnitt 3), nicht beim Replica-Start.
  - `APP_ENV` ist `staging`/`prod`; `AUTH_STATIC_TOKENS` darf dann nicht gesetzt sein (fataler Startup-Fehler).
  - Secrets werden über das Zielsystem bereitgestellt (Secret-Store/Runtime-Env), nicht im Image oder Repository.