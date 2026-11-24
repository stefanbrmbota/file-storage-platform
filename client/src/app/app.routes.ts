import { Routes } from '@angular/router';
import { Shell } from './layout/shell';

/**
 * Rute klijenta (T-32.2). Sve stranice su deca rasporeda Shell, pa dobijaju
 * zajednicko zaglavlje i navigaciju. Stranice se ucitavaju tek kad zatrebaju
 * (loadComponent), da pocetno ucitavanje ostane malo.
 *
 * Nova stranica: dodaj child rutu ispred '**' i stavku u Shell.nav.
 */
export const routes: Routes = [
  {
    path: '',
    component: Shell,
    children: [
      {
        path: '',
        title: 'Početna | File Storage',
        loadComponent: () => import('./pages/home/home').then((m) => m.Home),
      },
      {
        // Interceptor ovde salje korisnika kad token istekne; formu pravi T-32.4.
        path: 'prijava',
        title: 'Prijava | File Storage',
        loadComponent: () => import('./pages/login/login').then((m) => m.Login),
      },
      {
        path: '**',
        title: 'Stranica ne postoji | File Storage',
        loadComponent: () => import('./pages/not-found/not-found').then((m) => m.NotFound),
      },
    ],
  },
];
