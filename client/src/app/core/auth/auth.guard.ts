import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { map } from 'rxjs';
import { TokenService } from './token.service';
import { AuthService } from './auth.service';
import { LOGIN_PATH } from '../http/auth.interceptor';

/**
 * Zasticena ruta (T-32.4): bez vazeceg tokena korisnik ide na prijavu, uz
 * returnUrl da se posle prijave vrati na trazenu stranicu.
 *
 * Sacuvan token se jednom proverava kod gateway-a (GET /api/auth/session), pa
 * ni stari token posle odjave ne otvara stranicu. Ako gateway nije dostupan,
 * stranica se otvara, a traka prikazuje gresku; podatke svakako stiti gateway.
 */
export const authGuard: CanActivateFn = (_route, state) => {
  const router = inject(Router);
  const toLogin = () => router.createUrlTree([LOGIN_PATH], { queryParams: { returnUrl: state.url } });

  if (!inject(TokenService).get()) {
    return toLogin();
  }
  return inject(AuthService)
    .checkSession()
    .pipe(map((result) => (result === 'invalid' ? toLogin() : true)));
};

/** Stranica za prijavu: prijavljen korisnik se odmah vraca na pocetnu. */
export const guestGuard: CanActivateFn = () => {
  if (!inject(TokenService).get()) {
    return true;
  }
  return inject(Router).createUrlTree(['/']);
};
