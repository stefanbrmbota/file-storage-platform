# Klijent (Angular): kako dodati svoju stranicu

Klijent je Angular aplikacija u folderu `client/` (US-32). Osnova je gotova:
raspored sa zaglavljem, rutiranje, prijava i odjava, čuvanje tokena i
prikaz grešaka. Svako na nju dodaje stranice za svoj servis
(npr. SB foldere i datoteke, SC registraciju i admin).

Pravilo: **dodaješ samo svoju stranicu, osnovu ne menjaš.** Fajlovi u
`core/` i `layout/` su zajednički; ako ti tu nešto treba, javi AP-u.

## 1. Pokretanje

Bez Docker-a (brže dok praviš stranicu), iz foldera `client/`:

```
npm install
npx ng serve
```

Klijent je na http://localhost:3000 i sam se osvežava kad sačuvaš fajl.
Gateway mora da radi na `localhost:8080` (npr. `docker compose up -d` iz
root-a); adresu klijent čita iz `client/public/config.json`.

Sa Docker-om (kao na odbrani), iz root-a repoa:

```
docker compose up --build -d
```

Testovi, iz `client/`:

```
npx ng test --watch=false
```

Za prijavu treba potvrđen nalog; kod za prijavu (OTP) stiže u MailHog
(http://localhost:8025).

## 2. Struktura

```
client/src/app/
  app.routes.ts        sve rute (ovde dodaješ svoju)
  layout/shell.*       zaglavlje i navigacija (ovde dodaješ stavku menija)
  core/                zajedničko: config, token, prijava, interceptor, greške
  pages/<stranica>/    po jedan folder za svaku stranicu
    <stranica>.ts      logika
    <stranica>.html    šablon
    <stranica>.css     stil samo te stranice
    <stranica>.spec.ts testovi
```

Primer gotove zaštićene stranice: `pages/primer/` (ruta `/primer`).

## 3. Nova stranica korak po korak

Primer: stranica **Folderi** na adresi `/folderi`.

### 3.1 Napravi komponentu

Iz `client/`:

```
npx ng generate component pages/folderi
```

Nastaju `folderi.ts`, `folderi.html`, `folderi.css` i `folderi.spec.ts` u
`src/app/pages/folderi/`.

### 3.2 Dodaj rutu

U `src/app/app.routes.ts`, u niz `children`, **ispred** rute `'**'` (404):

```ts
{
  path: 'folderi',
  canActivate: [authGuard],          // zaštićena: bez prijave vodi na /prijava
  title: 'Folderi | File Storage',   // naslov taba
  loadComponent: () => import('./pages/folderi/folderi').then((m) => m.Folderi),
},
```

- Stranica koja traži prijavu (skoro sve) ima `canActivate: [authGuard]`.
- Javna stranica (npr. registracija, potvrda naloga) **nema** `canActivate`.
  Ako prijavljen korisnik ne treba da je vidi, stavi `canActivate: [guestGuard]`.
- Ruta mora biti ispred `'**'`, inače je uhvati stranica 404.

### 3.3 Dodaj stavku u meni (ako treba)

U `src/app/layout/shell.ts`, u niz `nav`:

```ts
protected readonly nav: NavItem[] = [
  { label: 'Početna', path: '/' },
  { label: 'Folderi', path: '/folderi' },
];
```

Meni se vidi samo prijavljenom korisniku.

### 3.4 Pozovi gateway

```ts
import { Component, OnInit, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';

// (@Component(...) iznad klase ostaje kako ga je napravio ng generate)
export class Folderi implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  protected readonly error = signal<string | null>(null);

  ngOnInit(): void {
    const folderId = '...'; // npr. iz rute
    this.http.get(`${this.config.gatewayUrl}/api/folders/${folderId}/contents`).subscribe({
      next: (res) => { /* prikaži podatke */ },
      error: (err) => this.error.set(apiErrorMessage(err)),
    });
  }
}
```

Šta osnova radi umesto tebe:

| Ne radiš ručno | Ko to radi |
|---|---|
| `Authorization: Bearer <token>` | interceptor dodaje sam, samo na zahteve ka gateway-u |
| istekao ili opozvan token (401) | interceptor briše token i vodi na `/prijava`, pa se korisnik vraća na tvoju stranicu |
| server ili mreža ne radi (5xx, 0) | crvena traka na vrhu stranice |
| adresa gateway-a | `config.gatewayUrl` (iz `config.json`) |

Tvoje su samo greške koje zavise od stranice (400, 403, 404, 409...):
prikaži `apiErrorMessage(err)`, koji vraća poruku servisa iz
`{"error": "..."}` ili opštu poruku.

Pravila (iz CLAUDE.md, važe i u klijentu):
- adresa gateway-a samo preko `config.gatewayUrl`, nikad zakucana;
- lozinke, tokeni i kodovi nikad u URL-u (ni u `?query`) ni u `console.log`;
- token ne čuvaj sam i ne šalji ručno, za to postoji `TokenService` i interceptor.

### 3.5 Prijavljen korisnik

Ako stranici treba ko je prijavljen (npr. da prikaže admin dugme):

```ts
import { TokenService } from '../../core/auth/token.service';

protected readonly claims = inject(TokenService).claims; // { userId, role, expiresAt } ili null
```

```html
@if (claims()?.role === 'ADMIN') { <a routerLink="/admin">Admin</a> }
```

To je samo za prikaz; pravo pristupa uvek proverava gateway i servis.

### 3.6 Stil

Koristi zajedničke boje iz `src/styles.css` (`var(--accent)`,
`var(--danger)`, `var(--border)`, `var(--muted)`...), da sve stranice
izgledaju isto. Stil samo za tvoju stranicu ide u njen `.css`.

### 3.7 Test

U `<stranica>.spec.ts` gateway se zamenjuje lažnim (`HttpTestingController`).
Gotov primer je `pages/primer/primer.spec.ts`: proverava i uspešan odgovor i
prikaz greške. Pre commit-a: `npx ng test --watch=false`.

## 3.8 Registracija i potvrda naloga (US-04, T-04.5)

Javne stranice (bez prijave):

| Ruta | Stranica | Šta radi |
|---|---|---|
| `/registracija` | `pages/registracija/` | Forma (ime, prezime, email, lozinka, ponovljena lozinka); pravila lozinke su ista kao u Users servisu; greške servera se prikazuju u formi; posle uspeha poruka "Proverite mejl". |
| `/potvrda` | `pages/potvrda/` | Ovde vodi link iz mejla: `/potvrda#token=...`. Stranica pročita token iz fragmenta, ukloni ga iz adrese i pošalje `POST /api/auth/confirm` (token u telu). |

Adresu linka određuje `CONFIRM_URL_BASE` u `services/users/.env.example`
(`http://localhost:3000/potvrda`); mejl sa linkom se vidi u MailHog-u
(http://localhost:8025). Ako promeniš rutu `/potvrda`, promeni i tu
promenljivu.

## 4. Provera pre commit-a

1. `npx ng test --watch=false` prolazi.
2. `npx ng build` prolazi bez grešaka.
3. U pretraživaču: stranica se otvara posle prijave, a bez prijave vodi na
   `/prijava` (za zaštićenu stranicu).
4. Menjao si samo `pages/<tvoja-stranica>/`, `app.routes.ts` i po potrebi
   `layout/shell.ts` (stavka menija).
