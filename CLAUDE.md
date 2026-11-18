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
`docker-compose.yml` u root-u podiže gateway, svih 5 servisa, MongoDB, Cassandru, Neo4j, MinIO i MailHog:
```
docker compose up --build -d
docker compose ps
docker compose logs -f <servis>
docker compose down
```
(`docker compose down -v` briše i podatke iz volume-a.)

Pravila kad menjaš servis ili compose:
- Spolja je dostupan SAMO gateway (`localhost:8080`) i MailHog UI (`127.0.0.1:8025`, samo razvoj). Servisi i baze su u mreži `backend` (`internal: true`) i NE dobijaju `ports:`; ne dodavati ih.
- U Docker mreži se ide po imenu servisa iz compose-a (`http://users:8081`, `mongo:27017`, `cassandra:9042`, `bolt://neo4j:7687`, `minio:9000`, `mailhog:1025`), ne `localhost`.
- Konfiguracija servisa: `<servis>/.env.example` pa opciono `<servis>/.env`; `environment:` u compose-u prepisuje samo adrese za Docker mrežu. Nova env promenljiva ide u `.env.example` servisa, a u compose samo ako je adresa drugog kontejnera.
- Tajne koje deli više kontejnera (`JWT_SECRET`, MinIO, Neo4j) su u root `.env` (šablon `.env.example` u root-u).
- Healthcheck (T-33.3): svaki kontejner ga ima, a svaki `depends_on` koristi `condition: service_healthy`. Redosled je baze → servisi → gateway. Go image-i su distroless (bez curl-a i shell-a), pa se proverava programom `/app/healthcheck <port>` iz `pkg/healthcheck`, koji zove `GET /health`. Zato svaki Go servis MORA imati `GET /health`, a novi Dockerfile se pravi iz `docker/Dockerfile.template`, koji već gradi i kopira healthcheck.
- Kad servis počne stvarno da koristi bazu ili drugi servis, dodaj taj kontejner u njegov `depends_on` sa `service_healthy`.
- Podaci baza su u imenovanim volume-ima (`mongo-data`, `cassandra-data`, `neo4j-data`, `minio-data`) i ostaju posle `docker compose down`; briše ih samo `down -v`.
- Git Bash na Windows-u: za `docker compose exec <servis> /app/...` prvo `export MSYS_NO_PATHCONV=1`, inače se putanja pretvori u Windows putanju.
- Lokalni rad bez Docker-a (`go run ./cmd/server`) i dalje koristi `localhost` adrese iz `.env.example`.
