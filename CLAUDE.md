# file-storage-platform

Mikroservisna platforma za skladištenje i deljenje fajlova. Studentski projekat (SOA/NoSQL + Informaciona bezbednost), tim od 3 člana.
Backlog: Trello. Svaki story ima oznaku `US-xx` i kriterijume prihvatanja; implementiraj tačno po njima.

## Tim i vlasništvo
- **SB (Stefan Brmbota)**: `services/files` (folderi, datoteke, object store, pretraga, broker, otpornost, saga)
- **SC (Sergej Čišić)**: `services/users` (registracija, prijava, kvota, admin) + ceo IB deo
- **AP (Aleksandar Pijetlović)**: `services/notifications`, `services/sharing`, `services/analytics`, `api-gateway`, klijent, Docker/K8s

## Struktura
```
api-gateway/            jedina ulazna tačka (REST), servisi nisu dostupni spolja
services/<servis>/      svaki servis je zaseban Go modul
  cmd/server/main.go    ulazna tačka
  internal/config/      konfiguracija iz env promenljivih (bez zakucanih adresa i lozinki)
  internal/model/       entiteti
  internal/repository/  pristup bazi
  internal/service/     poslovna logika
  internal/handler/     HTTP handleri
  Dockerfile
  .env.example
```

## Konvencije
- Go 1.27, standardni `net/http` ruter (`mux.HandleFunc("POST /folders", ...)`).
- Sloj: handler → service → repository. Handler ne priča direktno sa bazom.
- Konfiguracija samo iz env promenljivih; svaki servis ima `.env.example`. `.env` se ne commituje.
- Greške ka klijentu: JSON `{"error": "..."}`, bez stack trace-a i internih detalja (IB zahtev).
- Upiti ka bazi nikad ne spajaju stringove sa korisničkim ulazom.
- Lozinke, tokeni i OTP nikad u URL-u ni u logovima.
- Svaki poziv drugog servisa ima timeout (`context.WithTimeout`).
- Baze: Users/Files dokument-orijentisana (MongoDB), Notifications wide-column (Cassandra), Sharing graf (Neo4j). Sadržaj datoteka samo u object store-u (MinIO).

## Git
- Jedan story = jedan branch: `feature/US-xx-kratak-opis`, `fix/...`, `chore/...`.
- Commit poruke na srpskom, počinju oznakom story-ja: `US-01: kreiranje foldera`.
- U `main` samo preko PR-a uz review drugog člana.

## Komande
Iz foldera servisa (npr. `cd services/files`), svaka komanda posebno (Windows PowerShell 5.1 ne podržava `&&`):
```
go build ./...
go test ./...
go run ./cmd/server
```
