# file-storage-platform

Mikroservisna platforma za skladištenje i deljenje datoteka. Studentski
projekat (SOA/NoSQL + Informaciona bezbednost).

## Dokumentacija

- [Dizajn sistema](docs/design.md): modeli podataka svih servisa, mapa
  komunikacije (gateway, sinhroni pozivi, događaji) i otvorene odluke.
- [Izbor heširanja lozinki](docs/izbor-hesiranja-lozinki.md)

## Servisi

| Servis | Folder | Skladište |
|---|---|---|
| API gateway | `api-gateway/` | — |
| Users | `services/users/` | MongoDB |
| Files | `services/files/` | MongoDB + MinIO |
| Sharing | `services/sharing/` | Neo4j |
| Notifications | `services/notifications/` | Cassandra |
| Analytics | `services/analytics/` | Cassandra |

Spolja je dostupan samo API gateway. Svaki servis je zaseban Go modul, a
konfiguraciju čita iz env promenljivih (primer je u `.env.example` servisa).

## Pokretanje cele aplikacije (Docker Compose)

Jedna komanda podiže gateway, svih pet servisa i sve baze. Ovo je
preporučen način da se aplikacija pokrene.

### Preduslovi

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (uključen
  pre pokretanja komandi)
- Git
- Slobodni portovi `8080` i `8025` na računaru

### Prvo pokretanje

Iz root-a repozitorijuma:

```
docker compose up --build -d
```

Prvi put traje nekoliko minuta (preuzimanje image-a i build servisa).
Servisi se podižu redom: prvo baze, pa servisi kad je njihova baza spremna,
pa gateway kad su svi servisi spremni. Stanje se prati sa:

```
docker compose ps
```

Sve je spremno kada svih 11 kontejnera ima status `(healthy)`. Provera:

```
curl http://localhost:8080/health
```

Očekivan odgovor je `{"status":"ok"}`.

### Portovi

Spolja (sa računara) su dostupni samo:

| Adresa | Šta je |
|---|---|
| `http://localhost:8080` | API gateway, jedina ulazna tačka aplikacije |
| `http://localhost:8025` | MailHog, pregled mejlova koje aplikacija šalje (potvrda registracije, OTP); samo za razvoj |

Servisi i baze su u internoj Docker mreži i namerno **nisu** dostupni sa
računara. Međusobno se pozivaju po imenu kontejnera:

| Kontejner | Port u mreži | Koristi ga |
|---|---|---|
| `users` | 8081 | gateway, files, sharing |
| `files` | 8082 | gateway, users, analytics |
| `notifications` | 8083 | gateway |
| `sharing` | 8084 | gateway, files |
| `analytics` | 8085 | gateway |
| `mongo` | 27017 | users, files |
| `cassandra` | 9042 | notifications, analytics |
| `neo4j` | 7687 | sharing |
| `minio` | 9000 | files |
| `mailhog` | 1025 (SMTP) | users |

Ako je port `8080` zauzet, gateway se može objaviti na drugom portu preko
`GATEWAY_PUBLIC_PORT` u root `.env` fajlu (npr. `GATEWAY_PUBLIC_PORT=9080`).

### `.env` fajlovi

Za lokalno pokretanje **nije potreban nijedan `.env` fajl**: compose koristi
vrednosti iz `.env.example` fajlova. `.env` fajlovi služe samo kada želiš
svoje vrednosti i nikad se ne commituju.

| Fajl | Šta sadrži | Kada ga praviti |
|---|---|---|
| `.env` (root) | Tajne koje deli više kontejnera: `JWT_SECRET` (users potpisuje, gateway proverava), `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD`, `NEO4J_PASSWORD`, `GATEWAY_PUBLIC_PORT` | Kad menjaš tajne ili port gateway-a. Šablon: `.env.example` u root-u |
| `<servis>/.env` | Podešavanja jednog servisa (npr. `services/users/.env`: Argon2 parametri, rokovi OTP-a i JWT-a, admin nalog, kvota) | Kad menjaš ponašanje jednog servisa. Šablon: `.env.example` u folderu servisa |

Pravljenje root `.env` fajla (PowerShell):

```
Copy-Item .env.example .env
```

Napomene:

- Adrese baza i servisa (`mongo:27017`, `http://users:8081`...) postavlja
  `docker-compose.yml` i ne treba ih menjati u `.env` fajlovima.
- `JWT_SECRET` mora imati bar 32 karaktera, a lozinke za MinIO i Neo4j bar 8.
- Neo4j lozinku pamti pri prvom pokretanju. Ako je kasnije promeniš u `.env`,
  podatke Neo4j-a treba obrisati (`docker compose down -v`), inače se
  `sharing` neće povezati.
- Vrednosti `change-me...` su samo za lokalni razvoj.

### Korisne komande

```
docker compose logs -f users        # logovi jednog servisa
docker compose restart users        # restart jednog servisa
docker compose up --build -d users  # ponovni build posle izmene koda
docker compose down                 # gašenje, podaci ostaju
docker compose down -v              # gašenje i brisanje svih podataka
```

Podaci baza su u Docker volume-ima i ostaju posle `docker compose down`.

### Česti problemi

- **`docker compose` javlja da ne može da se poveže na Docker:** Docker
  Desktop nije pokrenut.
- **`port is already allocated` za 8080:** port je zauzet, promeni
  `GATEWAY_PUBLIC_PORT` (vidi gore) ili ugasi program koji ga koristi.
- **Servis ostaje `(health: starting)` duže od minut-dva:** obično čeka
  Cassandru, koja se sporo podiže pri prvom pokretanju. Logovi:
  `docker compose logs cassandra`.
- **Servis je `unhealthy` ili se stalno restartuje:**
  `docker compose logs <servis>` prikazuje razlog (najčešće neispravna
  vrednost u `.env` fajlu).
- **Git Bash pretvara putanje kao `/app/...` u Windows putanje** kod
  `docker compose exec`: pre komande pokreni `export MSYS_NO_PATHCONV=1`.

## Pokretanje servisa lokalno (bez Docker-a)

Za rad na jednom servisu, bez pokretanja cele aplikacije.

MongoDB za lokalni razvoj (jednom; posle toga `docker start files-mongo`):

```
docker run -d --name files-mongo -p 27017:27017 mongo:8.0
```

Iz foldera servisa (npr. `services/files`):

```
go build ./...
go test ./...
go run ./cmd/server
```
