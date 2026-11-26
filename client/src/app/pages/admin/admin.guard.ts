import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { TokenService } from '../../core/auth/token.service';

/**
 * Stranica samo za administratora (US-22, T-22.4): ko nije administrator vraca se
 * na pocetnu. Uloga se cita iz tokena, pa je ovo samo olaksanje za korisnika
 * (da ne vidi stranicu koja bi mu vratila gresku); pravo pristupa stvarno
 * proveravaju gateway i Users servis na svakom zahtevu.
 * Koristi se uz authGuard: canActivate: [authGuard, adminGuard].
 */
export const adminGuard: CanActivateFn = () => {
  if (inject(TokenService).claims()?.role === 'ADMIN') {
    return true;
  }
  return inject(Router).createUrlTree(['/']);
};
