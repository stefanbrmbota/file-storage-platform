import { Routes } from '@angular/router';
import { Shell } from './layout/shell';
import { authGuard, guestGuard } from './core/auth/auth.guard';

/**
 * Rute klijenta (T-32.2). Sve stranice su deca rasporeda Shell, pa dobijaju
 * zajednicko zaglavlje i navigaciju. Stranice se ucitavaju tek kad zatrebaju
 * (loadComponent), da pocetno ucitavanje ostane malo.
 *
 * Nova stranica: dodaj child rutu ispred '**' i stavku u Shell.nav. Stranica
 * koja trazi prijavu dobija canActivate: [authGuard]. Uputstvo: docs/klijent.md.
 */
export const routes: Routes = [
  {
    path: '',
    component: Shell,
    children: [
      {
        // Zasticena stranica: bez prijave vodi na /prijava (T-32.4).
        path: '',
        canActivate: [authGuard],
        title: 'Početna | File Storage',
        loadComponent: () => import('./pages/home/home').then((m) => m.Home),
      },
      {
        // Prijava u dva koraka; prijavljen korisnik se vraca na pocetnu.
        path: 'prijava',
        canActivate: [guestGuard],
        title: 'Prijava | File Storage',
        loadComponent: () => import('./pages/login/login').then((m) => m.Login),
      },
      {
        // Registracija (T-04.5); prijavljen korisnik se vraca na pocetnu.
        path: 'registracija',
        canActivate: [guestGuard],
        title: 'Registracija | File Storage',
        loadComponent: () => import('./pages/registracija/registracija').then((m) => m.Registracija),
      },
      {
        // Potvrda naloga: ovde vodi link iz mejla (/potvrda#token=...); javna, bez guard-a.
        path: 'potvrda',
        title: 'Potvrda naloga | File Storage',
        loadComponent: () => import('./pages/potvrda/potvrda').then((m) => m.Potvrda),
      },
      {
        // Zaboravljena lozinka (US-15, T-15.5): unos emaila; prijavljen korisnik se vraca na pocetnu.
        path: 'zaboravljena-lozinka',
        canActivate: [guestGuard],
        title: 'Zaboravljena lozinka | File Storage',
        loadComponent: () => import('./pages/zaboravljena-lozinka/zaboravljena-lozinka').then((m) => m.ZaboravljenaLozinka),
      },
      {
        // Reset lozinke: ovde vodi link iz mejla (/reset-lozinke#token=...); javna, bez guard-a.
        path: 'reset-lozinke',
        title: 'Nova lozinka | File Storage',
        loadComponent: () => import('./pages/reset-lozinke/reset-lozinke').then((m) => m.ResetLozinke),
      },
      {
        // Povracaj naloga magicnim linkom (US-17, T-17.5): unos emaila; prijavljen korisnik se vraca na pocetnu.
        path: 'povracaj-naloga',
        canActivate: [guestGuard],
        title: 'Povraćaj naloga | File Storage',
        loadComponent: () => import('./pages/povracaj-naloga/povracaj-naloga').then((m) => m.PovracajNaloga),
      },
      {
        // Prijava magicnim linkom: ovde vodi link iz mejla (/magicni-link#token=...); javna, bez guard-a.
        path: 'magicni-link',
        title: 'Prijava linkom | File Storage',
        loadComponent: () => import('./pages/magicni-link/magicni-link').then((m) => m.MagicniLink),
      },
      {
        // Profil i kvota skladista (US-06, T-06.3); zasticena stranica.
        path: 'profil',
        canActivate: [authGuard],
        title: 'Profil | File Storage',
        loadComponent: () => import('./pages/profil/profil').then((m) => m.Profil),
      },
      {
        // Promena lozinke (US-14, T-14.4); zasticena stranica, link je na profilu.
        path: 'promena-lozinke',
        canActivate: [authGuard],
        title: 'Promena lozinke | File Storage',
        loadComponent: () => import('./pages/promena-lozinke/promena-lozinke').then((m) => m.PromenaLozinke),
      },
      {
        // PRIMER zasticene stranice za kopiranje (T-32.5, docs/klijent.md); nije u navigaciji.
        path: 'primer',
        canActivate: [authGuard],
        title: 'Primer | File Storage',
        loadComponent: () => import('./pages/primer/primer').then((m) => m.Primer),
      },
      {
        // Folderi i datoteke (SB): /folderi je korenski folder, /folderi/:id bilo koji drugi.
        path: 'folderi',
        canActivate: [authGuard],
        title: 'Moji fajlovi | File Storage',
        loadComponent: () => import('./pages/folderi/folderi').then((m) => m.Folderi),
      },
      {
        path: 'folderi/:id',
        canActivate: [authGuard],
        title: 'Moji fajlovi | File Storage',
        loadComponent: () => import('./pages/folderi/folderi').then((m) => m.Folderi),
      },
      {
        // Obavestenja prijavljenog korisnika (US-08, T-08.2); zasticena stranica.
        path: 'obavestenja',
        canActivate: [authGuard],
        title: 'Obaveštenja | File Storage',
        loadComponent: () => import('./pages/obavestenja/obavestenja').then((m) => m.Obavestenja),
      },
      {
        path: '**',
        title: 'Stranica ne postoji | File Storage',
        loadComponent: () => import('./pages/not-found/not-found').then((m) => m.NotFound),
      },
    ],
  },
];
