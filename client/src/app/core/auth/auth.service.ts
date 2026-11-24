import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Router } from '@angular/router';
import { Observable, catchError, finalize, map, of } from 'rxjs';
import { ConfigService } from '../config.service';
import { TokenService } from './token.service';
import { LOGIN_PATH } from '../http/auth.interceptor';

/**
 * Prijava u dva koraka i odjava preko gateway-a (US-32, T-32.4):
 *  1. POST /api/auth/login {email, password} -> kod (OTP) stize mejlom;
 *  2. POST /api/auth/login/verify {email, code} -> {token}, koji se cuva;
 *  odjava: POST /api/auth/logout -> Users opoziva token, klijent ga brise.
 * Lozinka i kod se nigde ne cuvaju i ne idu u URL.
 */
@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);
  private readonly tokens = inject(TokenService);
  private readonly router = inject(Router);

  /** Token za koji je gateway vec potvrdio da vazi (da se ne pita pri svakom prelasku). */
  private verifiedToken: string | null = null;

  private url(path: string): string {
    return `${this.config.gatewayUrl}${path}`;
  }

  /** Prvi korak: email i lozinka. Uspeh znaci da je kod poslat mejlom. */
  login(email: string, password: string): Observable<void> {
    return this.http.post<unknown>(this.url('/api/auth/login'), { email, password }).pipe(map(() => undefined));
  }

  /** Drugi korak: kod iz mejla. Uspeh cuva token i korisnik je prijavljen. */
  verify(email: string, code: string): Observable<void> {
    return this.http.post<{ token: string }>(this.url('/api/auth/login/verify'), { email, code }).pipe(
      map((res) => {
        if (!res || typeof res.token !== 'string') {
          throw new Error('odgovor nema token');
        }
        this.tokens.set(res.token);
        if (!this.tokens.get()) {
          throw new Error('token nije ispravan');
        }
      }),
    );
  }

  /**
   * Proverava kod gateway-a da li sacuvan token jos vazi (potpis, rok, opoziv).
   * Pita se jednom po tokenu. Vraca 'valid', 'invalid' (401: token istekao ili
   * opozvan, npr. odjava u drugom tabu) ili 'unknown' (gateway nedostupan).
   */
  checkSession(): Observable<'valid' | 'invalid' | 'unknown'> {
    const token = this.tokens.get();
    if (!token) {
      return of('invalid');
    }
    if (token === this.verifiedToken) {
      return of('valid');
    }
    return this.http.get<unknown>(this.url('/api/auth/session')).pipe(
      map(() => {
        this.verifiedToken = token;
        return 'valid' as const;
      }),
      catchError((err: unknown) => {
        if (err instanceof HttpErrorResponse && err.status === 401) {
          this.tokens.clear(); // opozvan ili istekao: vise ga ne koristimo
          return of('invalid' as const);
        }
        return of('unknown' as const);
      }),
    );
  }

  /**
   * Odjava: gateway/Users opoziva token, pa vise ne vazi ni ako ga neko sacuva.
   * Token se lokalno brise i kad poziv ne uspe, a korisnik ide na prijavu.
   */
  logout(): Observable<void> {
    return this.http.post<void>(this.url('/api/auth/logout'), null).pipe(
      map(() => undefined),
      finalize(() => {
        this.verifiedToken = null;
        this.tokens.clear();
        void this.router.navigate([LOGIN_PATH]);
      }),
    );
  }
}
