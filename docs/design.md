# Dizajn sistema

Dokument opisuje arhitekturu platforme (US-29). Popunjava se po podtaskovima:

| Task | Sekcija | Stanje |
|---|---|---|
| T-29.1 | [Model podataka – Users i Files](#1-model-podataka--users-i-files) | gotovo |
| — | Model podataka – Sharing, Notifications, Analytics | sledi |
| — | Mapa komunikacije (sinhroni/asinhroni pozivi, endpointi, događaji) | sledi |

---

## 1. Model podataka – Users i Files

Oba servisa koriste **MongoDB** (dokument-orijentisana baza), svaki svoju bazu
(`USERS_MONGO_DB=users`, `FILES_MONGO_DB=files`). Servisi ne čitaju tuđu bazu;
podaci drugog servisa dobijaju se samo preko njegovog API-ja. Sadržaj datoteka
nije u MongoDB-u nego u object store-u (**MinIO**, bucket `MINIO_BUCKET`).

Oznake u tabelama:
- **Obavezno**: polje uvek postoji i ima smislenu vrednost.
- **Ka klijentu**: da li se polje serijalizuje u JSON odgovor (`json:"-"` = ne).
- *Planirano*: polje postoji u modelu, ali ga kod još ne koristi.

### 1.1 Pregled

```
 Users servis (baza: users)                 Files servis (baza: files)
 ┌──────────────────────┐                   ┌──────────────────────┐
 │ users                │  _id = ownerId    │ folders              │◄─┐ parentId
 │  _id, email, role,   │──────────────────►│  _id, ownerId,       │──┘ (stablo)
 │  quotaBytes,         │   (logička veza,  │  parentId, name      │
 │  usedBytes, ...      │    bez FK)        └──────────▲───────────┘
 └───┬──────────────┬───┘                              │ parentId
     │ userId       │ _id = userId        ┌────────────┴─────────┐
 ┌───▼──────────┐ ┌─▼──────────┐          │ files (metapodaci)   │
 │ confirmation │ │ otps       │          │  _id, ownerId,       │
 │ _tokens (TTL)│ │ (TTL)      │          │  parentId, size,     │
 └──────────────┘ └────────────┘          │  objectKey ──────────┼──► MinIO objekat
                                          └──────────────────────┘    (sadržaj)
```

### 1.2 Users servis

#### Kolekcija `users`

Model: `services/users/internal/model/user.go` (`model.User`).

| Polje (BSON) | Go tip | BSON tip | Obavezno | Ka klijentu | Opis |
|---|---|---|---|---|---|
| `_id` | `string` | string | da | da (`id`) | 32 heks znaka (16 nasumičnih bajtova iz `crypto/rand`). Nije sekvencijalan, pa se ne može pogađati. |
| `firstName` | `string` | string | da | da | Ime. |
| `lastName` | `string` | string | da | da | Prezime. |
| `email` | `string` | string | da | da | Čuva se normalizovan (`trim` + mala slova), pa je jedinstvenost praktično neosetljiva na velika/mala slova. |
| `passwordHash` | `string` | string | da | **ne** | Argon2id u PHC formatu (`$argon2id$v=19$m=...,t=...,p=...$salt$hash`); parametri iz env-a. Vidi `docs/izbor-hesiranja-lozinki.md`. |
| `role` | `model.Role` | string | da | da | `USER` ili `ADMIN`. Registracija uvek daje `USER`; admin nastaje samo pri startu servisa (US-24). |
| `active` | `bool` | bool | da | da | `false` dok korisnik ne potvrdi registraciju mejlom (US-04); admin je odmah aktivan. |
| `quotaBytes` | `int64` | long | da | da | Dozvoljeni prostor u bajtovima; početno `USERS_DEFAULT_QUOTA_BYTES` (1 GiB). |
| `usedBytes` | `int64` | long | da | da | Zauzeti prostor; početno 0. Mora važiti `0 ≤ usedBytes ≤ quotaBytes`. |
| `passwordChangedAt` | `time.Time` | date (UTC) | da | da | Trenutak poslednje promene lozinke; pri registraciji = `createdAt`. *Planirano*: JWT izdat pre ovog trenutka (`iat < passwordChangedAt`) smatra se nevažećim. |
| `createdAt` | `time.Time` | date (UTC) | da | da | Trenutak kreiranja naloga. |

Indeksi:

| Ime | Ključ | Opcije | Upit kome služi |
|---|---|---|---|
| `_id_` | `_id` | podrazumevani | `FindByID`, `Activate`, `Delete` |
| `uniq_email` | `{ email: 1 }` | `unique` | `FindByEmail` (prijava); baza sama odbija dupli mejl, čak i kod istovremenih registracija |

#### Kolekcija `confirmation_tokens`

Model: `services/users/internal/model/confirmation_token.go`. Jednokratni token
iz linka za potvrdu registracije (US-04).

| Polje (BSON) | Go tip | BSON tip | Obavezno | Ka klijentu | Opis |
|---|---|---|---|---|---|
| `_id` | `string` | string | da | ne | SHA-256 heš tokena (heks). Sam token se ne čuva, pa procurela baza ne daje upotrebljive linkove. |
| `userId` | `string` | string | da | ne | `users._id` naloga koji se potvrđuje. |
| `expiresAt` | `time.Time` | date (UTC) | da | ne | Kraj važenja (`CONFIRMATION_TTL`, podrazumevano 24h). |

Indeksi:

| Ime | Ključ | Opcije | Svrha |
|---|---|---|---|
| `_id_` | `_id` | podrazumevani | Potrošnja tokena po hešu jednim `FindOneAndDelete`, pa je token atomično jednokratan |
| `ttl_expiresAt` | `{ expiresAt: 1 }` | `expireAfterSeconds: 0` | MongoDB sam briše istekle tokene |

#### Kolekcija `otps`

Model: `services/users/internal/model/otp.go`. Jednokratni kod za prijavu (US-05).

| Polje (BSON) | Go tip | BSON tip | Obavezno | Ka klijentu | Opis |
|---|---|---|---|---|---|
| `_id` | `string` | string | da | ne | `users._id`. Pošto je to ključ, korisnik ima najviše jedan aktivan OTP; novi zamenjuje stari. |
| `codeHash` | `string` | string | da | ne | HMAC koda (tajna `OTP_HMAC_SECRET`). Sam kod se ne čuva. |
| `expiresAt` | `time.Time` | date (UTC) | da | ne | Kraj važenja (`OTP_TTL`, podrazumevano 5 min). |
| `attempts` | `int` | int | da | ne | Broj neuspelih pokušaja. *Planirano*: posle limita OTP se poništava (zaštita od pogađanja). |

Indeksi:

| Ime | Ključ | Opcije | Svrha |
|---|---|---|---|
| `_id_` | `_id` | podrazumevani | Pronalaženje OTP-a za korisnika |
| `ttl_expiresAt` | `{ expiresAt: 1 }` | `expireAfterSeconds: 0` | Automatsko brisanje isteklih kodova |

> **Napomena o TTL-u:** MongoDB briše istekle dokumente u pozadini otprilike
> jednom u minuti, pa istekao token ili OTP može kratko ostati u bazi. Servis
> zato uvek sam proverava `expiresAt`; TTL indeks služi samo za čišćenje.

### 1.3 Files servis

#### Kolekcija `folders`

Model: `services/files/internal/model/model.go` (`model.Folder`). Folderi čine
stablo po korisniku (US-01).

| Polje (BSON) | Go tip | BSON tip | Obavezno | Ka klijentu | Opis |
|---|---|---|---|---|---|
| `_id` | `string` | string | da | da (`id`) | UUID v4. |
| `name` | `string` | string | da | da | Naziv foldera; jedinstven među folderima istog roditelja. Korenski folder se zove `root`. |
| `ownerId` | `string` | string | da | da | `users._id` vlasnika (veza ka Users servisu, bez FK). |
| `parentId` | `string` | string | da* | da (izostavlja se ako je prazan) | `_id` roditeljskog foldera. *Prazan string samo kod korenskog foldera. |
| `createdAt` | `time.Time` | date (UTC) | da | da | Trenutak kreiranja. |
| `updatedAt` | `time.Time` | date (UTC) | da | da | Poslednja izmena (npr. preimenovanje). |

Indeksi:

| Ime | Ključ | Opcije | Upit kome služi |
|---|---|---|---|
| `_id_` | `_id` | podrazumevani | `FindByID`, `Rename` |
| `ownerId_1_parentId_1_name_1` | `{ ownerId: 1, parentId: 1, name: 1 }` | `unique` | (1) baza odbija dva foldera istog imena u istom roditelju, i pri kreiranju i pri preimenovanju; (2) `ListByParent` (filter `ownerId`+`parentId`, sort po `name`) koristi isti indeks bez dodatnog sortiranja; (3) `FindRoot` (`ownerId`, `parentId: ""`) |

Korenski folder nastaje kada korisnik potvrdi registraciju: Users servis poziva
Files servis, a `CreateRoot` je idempotentan (ponovljen poziv vraća postojeći
koren). Unique indeks sa fiksnim imenom `root` dodatno sprečava dva korena.

#### Kolekcija `files`

Model: `services/files/internal/model/model.go` (`model.File`). Čuvaju se samo
metapodaci; sadržaj je u MinIO-u (US-02, US-12).

| Polje (BSON) | Go tip | BSON tip | Obavezno | Ka klijentu | Opis |
|---|---|---|---|---|---|
| `_id` | `string` | string | da | da (`id`) | UUID v4. |
| `name` | `string` | string | da | da | Naziv datoteke sa ekstenzijom; jedinstven među datotekama istog foldera. |
| `contentType` | `string` | string | da | da | MIME tip (npr. `application/pdf`). |
| `size` | `int64` | long | da | da | Veličina u bajtovima; ulazi u `users.usedBytes`. |
| `ownerId` | `string` | string | da | da | `users._id` vlasnika. |
| `parentId` | `string` | string | da | da | `folders._id` foldera u kom je datoteka (nikad prazan). |
| `objectKey` | `string` | string | da | **ne** | Ključ objekta u MinIO bucketu. Ne ide ka klijentu; preuzimanje uvek ide kroz servis, koji proverava prava. |
| `hash` | `string` | string | da | **ne** | Heš sadržaja (npr. SHA-256) za proveru integriteta posle otpremanja/preuzimanja. *Planirano*. |
| `createdAt` | `time.Time` | date (UTC) | da | da | Trenutak otpremanja. |
| `updatedAt` | `time.Time` | date (UTC) | da | da | Poslednja izmena. |

Indeksi:

| Ime | Ključ | Opcije | Upit kome služi |
|---|---|---|---|
| `_id_` | `_id` | podrazumevani | `FindByID`, `Rename` |
| `ownerId_1_parentId_1_name_1` | `{ ownerId: 1, parentId: 1, name: 1 }` | `unique` | Jedinstveno ime u folderu; `ListByParent` sortiran po imenu; prefiks `ownerId` pokriva i upite „sve datoteke korisnika“ (npr. preračun zauzeća) |

### 1.4 Veze između entiteta

| Veza | Kardinalnost | Gde se čuva | Kako se održava |
|---|---|---|---|
| `users` → `folders` | 1 : N (tačno jedan koren) | `folders.ownerId` | Koren se kreira pri potvrdi registracije (poziv Users → Files sa timeoutom `USERS_REQUEST_TIMEOUT`). |
| `folders` → `folders` | 1 : N (stablo) | `folders.parentId` | Files servis proverava da roditelj postoji i pripada istom vlasniku. |
| `folders` → `files` | 1 : N | `files.parentId` | Isto kao gore. |
| `users` → `files` | 1 : N | `files.ownerId` | Pri otpremanju i brisanju menja se i `users.usedBytes` (vidi ispod). |
| `files` → MinIO objekat | 1 : 1 | `files.objectKey` | Metapodatak bez objekta ili objekat bez metapodatka je greška; redosled upisa i kompenzacija su deo sage. |
| `users` → `confirmation_tokens` | 1 : N (u praksi 1) | `confirmation_tokens.userId` | Token se briše u istom koraku u kom se čita (`FindOneAndDelete`); neiskorišćeni ističu kroz TTL. |
| `users` → `otps` | 1 : 0..1 | `otps._id` | Novi OTP prepisuje stari. |

Pošto su `users` i `files`/`folders` u različitim bazama i servisima, **nema
stranih ključeva ni transakcija preko granice servisa**. Konzistentnost se
održava ovako:

- **Kvota** (`usedBytes`): otpremanje je raspodeljena operacija (rezervacija
  prostora u Users → upis objekta u MinIO → upis metapodataka u Files). Ako neki
  korak padne, prethodni se poništavaju kompenzacijom (saga, `services/files`).
- **Brisanje naloga**: brisanje korisnika mora da povuče brisanje njegovih
  foldera, datoteka i MinIO objekata; i to ide kroz sagu ili događaj preko brokera.
- Svaki poziv drugog servisa ima timeout (`context.WithTimeout`).

### 1.5 Bezbednosne odluke u modelu (IB)

- Lozinke, tokeni za potvrdu i OTP kodovi **nikad se ne čuvaju u čitljivom
  obliku**: lozinka kao Argon2id, token kao SHA-256, OTP kao HMAC.
- Polja `passwordHash`, `objectKey` i `hash` imaju `json:"-"` i ne mogu
  procureti u odgovor; to proverava test `TestPasswordHashNotSerialized`.
- ID-jevi korisnika su nasumični (128 bita), a folderi i datoteke imaju UUID v4,
  pa se tuđi resursi ne mogu nabrojati pogađanjem ID-ja. Svaki upit nad
  folderima i datotekama ipak filtrira i po `ownerId`.
- Uloga se ne može zadati kroz registraciju: DTO nema polje `role`, a
  `model.NewUser` uvek postavlja `USER`.
- Upiti koriste `bson.D` / `bson.M` sa vrednostima kao parametrima, bez spajanja
  stringova sa korisničkim ulazom (zaštita od NoSQL injekcije).

### 1.6 Predlozi (nije implementirano)

Sledeće se ne nalazi u kodu. Ovo su predlozi za vlasnike servisa (SB za Files,
SC za Users), uz story kome pripadaju:

| Kolekcija | Predlog | Razlog |
|---|---|---|
| `files` | tekstualni indeks `{ name: "text" }`, ili indeks `{ ownerId: 1, name: 1 }` za pretragu po prefiksu | Pretraga datoteka po nazivu; postojeći indeks počinje sa `parentId` posle `ownerId`, pa ne pomaže pretrazi kroz sve foldere. |
| `files` | unique indeks `{ objectKey: 1 }` | Garantuje da dva zapisa ne pokazuju na isti objekat (brisanje jednog bi uništilo drugi). |
| `folders` | polje `ancestors: [string]` (ID-jevi predaka) + indeks `{ ownerId: 1, ancestors: 1 }` | Brisanje ili premeštanje celog podstabla jednim upitom i provera da se folder ne premešta u sopstvenog potomka. |
| `users` | indeks `{ role: 1, createdAt: -1 }` | Admin pregled korisnika sa filtrom po ulozi i sortiranjem po datumu. |
| `users` | korišćenje `passwordChangedAt` pri validaciji JWT-a | Posle promene lozinke stari tokeni prestaju da važe. |
| `otps` | limit na `attempts` (npr. 5), posle čega se OTP briše | Sprečava pogađanje 6-cifrenog koda u roku važenja. |
| `files` | popunjavanje i provera `hash` | Otkrivanje oštećenog ili podmetnutog sadržaja u object store-u. |
