# Izbor algoritma za heširanje lozinki: Argon2id

Dokument obrazlaže zašto servis `users` čuva lozinke pomoću Argon2id, a ne pomoću MD5 ili SHA-256, i čemu služi so.

## 1. Problem

Ako baza korisnika procuri, napadač dobija sve što je u njoj. Zato se lozinke nikad ne čuvaju u čistom tekstu, već kao heš. Pri tome nije dovoljno da heš bude jednosmeran. Napadač neće "obrnuti" heš, već će nabrajati moguće lozinke, heširati ih i upoređivati rezultate (brute-force i rečnički napad). Zato funkcija za lozinke mora da bude **namerno spora i skupa** za svakoga ko pogađa, dok je za jednu legitimnu proveru pri prijavi to i dalje prihvatljiva cena.

## 2. Zašto ne MD5 i SHA-256

- **Brzina je mana.** MD5 i SHA-256 su projektovani da budu što brži (provera integriteta, potpisi). Savremena GPU i ASIC oprema računa milijarde takvih heševa u sekundi, pa je pogađanje slabih i srednjih lozinki jeftino.
- **MD5 je kriptografski razbijen.** Poznate su kolizije, pa se ne preporučuje ni u jednoj bezbednosnoj upotrebi.
- **Nema podesivog troška.** Kada hardver postane brži, ne može se "pojačati" cena računanja, a da se ne promeni sam algoritam.
- **Nisu memorijski zahtevni.** Mogu se masovno paralelizovati na hardveru s malo memorije po jezgru.

## 3. Zašto Argon2id

- Pobednik takmičenja Password Hashing Competition i preporuka OWASP-a (RFC 9106).
- **Memorijski zahtevan.** Svaki pokušaj traži zadati iznos RAM-a, što drastično ograničava paralelizaciju na GPU/ASIC uređajima.
- **Podesivi parametri:** memorija (`m`), broj iteracija (`t`) i paralelizam (`p`). Trošak se može povećavati s razvojem hardvera.
- **Varijanta "id"** kombinuje otpornost na side-channel napade (Argon2i) i na GPU napade (Argon2d), pa je preporučeni izbor za opštu upotrebu.

| Svojstvo | MD5 | SHA-256 | Argon2id |
|---|---|---|---|
| Namena | integritet | integritet, potpisi | čuvanje lozinki |
| Brzina | vrlo brz | vrlo brz | namerno spor |
| Memorijska zahtevnost | ne | ne | da (podesiva) |
| Podesiv trošak | ne | ne | da (`m`, `t`, `p`) |
| Status | razbijen | bezbedan, ali neprikladan za lozinke | preporučen |

## 4. Čemu služi so

So je **nasumična vrednost, jedinstvena za svakog korisnika**, koja se dodaje lozinki pre heširanja i čuva se uz heš. So nije tajna.

- Dva korisnika s istom lozinkom dobijaju **različite heševe**, pa se iz baze ne vidi ko deli lozinku.
- Onemogućava unapred izračunate **rainbow tabele**, jer bi se tabela morala praviti posebno za svaku so.
- Napadač ne može da jednim prolazom proverava kandidat-lozinku protiv svih naloga, već mora da radi poseban napor za svaki nalog.

Argon2id so prima kao ulaz i uobičajeno se čuva u istom enkodiranom zapisu zajedno s parametrima i hešom, pa nije potrebna posebna kolona.

## 5. Odluka za naš projekat

- Algoritam: **Argon2id**, biblioteka `golang.org/x/crypto/argon2`.
- So: **16 bajtova** iz `crypto/rand`, nova za svaku lozinku.
- Parametri (OWASP minimum): **m = 19 MiB, t = 2, p = 1**.
- Parametri se čitaju iz env promenljivih (konvencija projekta: bez zakucanih vrednosti) i čuvaju se uz heš, da bi se kasnije mogli povećati bez gubitka mogućnosti provere starih heševa.
- Poređenje heševa u konstantnom vremenu (`crypto/subtle`).
- Lozinke se nikad ne loguju niti pojavljuju u URL-u.

## 6. Izvori

- OWASP Password Storage Cheat Sheet
- RFC 9106: Argon2, Memory-Hard Function for Password Hashing and Proof-of-Work Applications
