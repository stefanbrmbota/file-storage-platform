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

## Pokretanje servisa lokalno

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
