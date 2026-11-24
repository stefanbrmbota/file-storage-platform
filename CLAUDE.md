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
MongoDB za lokalni razvoj (jednom; posle `docker start files-mongo`):
```
docker run -d --name files-mongo -p 27017:27017 mongo:8.0
```
MinIO za lokalni razvoj Files servisa (jednom; posle `docker start files-minio`; isti kredencijali kao u `services/files/.env.example`):
```
docker run -d --name files-minio -p 9000:9000 -e MINIO_ROOT_USER=change-me -e MINIO_ROOT_PASSWORD=change-me-minio cgr.dev/chainguard/minio@sha256:7618b82bba99aadd77fcf59eb6cfd89b4b8b8e01ac796c474d57a1c1abf17e79 server /data
```
MailHog za potvrdu registracije u razvoju (jednom; posle `docker start mailhog`; SMTP na 1025, UI na http://localhost:8025):
```
docker run -d --name mailhog -p 1025:1025 -p 8025:8025 mailhog/mailhog
```
Iz foldera servisa (npr. `cd services/files`), svaka komanda posebno (Windows PowerShell 5.1 ne podržava `&&`):
```
go build ./...
go test ./...
go run ./cmd/server
```
Docker image se gradi iz ROOT-a repoa (ne iz foldera servisa), jer servisi koriste `pkg/` preko `replace`. Šablon: `docker/Dockerfile.template`.
```
docker build -f services/<servis>/Dockerfile -t fsp-<servis> .
docker build -f api-gateway/Dockerfile -t fsp-gateway .
```

## Docker Compose (lokalno okruženje, US-33 / T-33.2, vlasnik AP)
`docker-compose.yml` u root-u podiže klijenta (Angular), gateway, svih 5 servisa, MongoDB, Cassandru, Neo4j, MinIO i MailHog:
```
docker compose up --build -d
docker compose ps
docker compose logs -f <servis>
docker compose down
```
(`docker compose down -v` briše i podatke iz volume-a.)

Pravila kad menjaš servis ili compose:
- Spolja su dostupni SAMO klijent (`localhost:3000`, mreža `public`, ne vidi servise), gateway (`localhost:8080`) i MailHog UI (`127.0.0.1:8025`, samo razvoj). Servisi i baze su u mreži `backend` (`internal: true`) i NE dobijaju `ports:`; ne dodavati ih.
- U Docker mreži se ide po imenu servisa iz compose-a (`http://users:8081`, `mongo:27017`, `cassandra:9042`, `bolt://neo4j:7687`, `minio:9000`, `mailhog:1025`), ne `localhost`.
- Konfiguracija servisa: `<servis>/.env.example` pa opciono `<servis>/.env`; `environment:` u compose-u prepisuje samo adrese za Docker mrežu. Nova env promenljiva ide u `.env.example` servisa, a u compose samo ako je adresa drugog kontejnera.
- Tajne koje deli više kontejnera (`JWT_SECRET`, MinIO, Neo4j) su u root `.env` (šablon `.env.example` u root-u).
- Healthcheck (T-33.3): svaki kontejner ga ima, a svaki `depends_on` koristi `condition: service_healthy`. Redosled je baze → servisi → gateway. Go image-i su distroless (bez curl-a i shell-a), pa se proverava programom `/app/healthcheck <port>` iz `pkg/healthcheck`, koji zove `GET /health`. Zato svaki Go servis MORA imati `GET /health`, a novi Dockerfile se pravi iz `docker/Dockerfile.template`, koji već gradi i kopira healthcheck.
- Kad servis počne stvarno da koristi bazu ili drugi servis, dodaj taj kontejner u njegov `depends_on` sa `service_healthy`.
- Podaci baza su u imenovanim volume-ima (`mongo-data`, `cassandra-data`, `neo4j-data`, `minio-data`) i ostaju posle `docker compose down`; briše ih samo `down -v`.
- Git Bash na Windows-u: za `docker compose exec <servis> /app/...` prvo `export MSYS_NO_PATHCONV=1`, inače se putanja pretvori u Windows putanju.
- Lokalni rad bez Docker-a (`go run ./cmd/server`) i dalje koristi `localhost` adrese iz `.env.example`.

## Klijent (Angular, US-32, vlasnik AP)
`client/` je Angular aplikacija na portu 3000; adresu gateway-a čita iz `/config.json` (u Docker-u ga pravi kontejner iz `GATEWAY_URL`, za `ng serve` je `client/public/config.json`). Iz `client/`:
```
npm install
npx ng serve
npx ng test --watch=false
npx ng build
```
Nova stranica: `pages/<stranica>/` (`npx ng generate component pages/<stranica>`), ruta u `app.routes.ts` ispred `'**'` (zaštićena sa `canActivate: [authGuard]`), stavka menija u `layout/shell.ts`. Gateway se zove preko `HttpClient` i `config.gatewayUrl`; token i 401 rešava interceptor, a greške `apiErrorMessage`. `core/` i `layout/` ne menjati bez dogovora sa AP. Detaljno: `docs/klijent.md`.
