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
        // PRIMER zasticene stranice za kopiranje (T-32.5, docs/klijent.md); nije u navigaciji.
        path: 'primer',
        canActivate: [authGuard],
        title: 'Primer | File Storage',
        loadComponent: () => import('./pages/primer/primer').then((m) => m.Primer),
      },
      {
        path: '**',
        title: 'Stranica ne postoji | File Storage',
        loadComponent: () => import('./pages/not-found/not-found').then((m) => m.NotFound),
      },
    ],
  },
];
