import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { ConfigService } from '../config.service';
import { TokenService } from '../auth/token.service';
import { ErrorBannerService } from '../notify/error-banner.service';
import { apiErrorMessage } from './api-error';

/** Putanja stranice za prijavu (T-32.4). */
export const LOGIN_PATH = '/prijava';

/** Provera sesije (GET /api/auth/session), koju zove guard. */
export const SESSION_PATH = '/api/auth/session';

/**
 * Rute gateway-a gde 401 znaci "pogresni podaci", a ne "token je istekao",
 * pa se korisnik ne salje na prijavu (vec je tamo).
 */
const PUBLIC_AUTH_PATHS = ['/api/auth/register', '/api/auth/confirm', '/api/auth/login', '/api/auth/login/verify'];

/**
 * Za svaki zahtev ka gateway-u (T-32.3):
 *  - dodaje Authorization: Bearer <token> ako je korisnik prijavljen; token
 *    nikad ne ide na druge adrese;
 *  - na 401 (token istekao ili opozvan odjavom) brise token i vodi na prijavu,
 *    uz returnUrl da se korisnik posle prijave vrati gde je bio;
 *  - greske servera i mreze (5xx, 0) prikazuje u traci na vrhu stranice.
 *    Ostale greske (400, 404, 409...) obradjuje stranica koja je zvala.
 */
export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const config = inject(ConfigService);
  const tokens = inject(TokenService);
  const router = inject(Router);
  const banner = inject(ErrorBannerService);

  const gateway = config.gatewayUrl;
  const toGateway = req.url === gateway || req.url.startsWith(gateway + '/');
  if (!toGateway) {
    return next(req);
  }

  const token = tokens.get();
  const authReq = token ? req.clone({ setHeaders: { Authorization: `Bearer ${token}` } }) : req;
  const path = req.url.slice(gateway.length).split('?')[0];

  return next(authReq).pipe(
    catchError((err: unknown) => {
      if (err instanceof HttpErrorResponse) {
        if (err.status === 401 && !PUBLIC_AUTH_PATHS.includes(path)) {
          tokens.clear();
          // Proveru sesije poziva guard, koji sam vodi na prijavu sa pravim returnUrl.
          if (path !== SESSION_PATH) {
            const returnUrl = router.url.startsWith(LOGIN_PATH) ? undefined : router.url;
            void router.navigate([LOGIN_PATH], { queryParams: returnUrl ? { returnUrl } : {} });
          }
        } else if (err.status === 0 || err.status >= 500) {
          banner.show(apiErrorMessage(err));
        }
      }
      return throwError(() => err);
    }),
  );
};
