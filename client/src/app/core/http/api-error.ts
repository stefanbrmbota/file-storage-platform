import { HttpErrorResponse } from '@angular/common/http';

/**
 * Poruka za korisnika iz greske gateway-a (T-32.3). Gateway i servisi vracaju
 * {"error": "..."}; ako te poruke nema, koristi se opsta poruka za status.
 */
export function apiErrorMessage(err: unknown): string {
  if (!(err instanceof HttpErrorResponse)) {
    return 'Došlo je do greške. Pokušaj ponovo.';
  }
  if (err.status === 0) {
    return 'Server trenutno nije dostupan. Proveri vezu i pokušaj ponovo.';
  }
  const body = err.error as { error?: unknown } | null;
  if (body && typeof body.error === 'string' && body.error.trim() !== '') {
    return body.error;
  }
  switch (err.status) {
    case 400:
      return 'Zahtev nije ispravan.';
    case 401:
      return 'Prijava je istekla. Prijavi se ponovo.';
    case 403:
      return 'Nemaš pravo na ovu akciju.';
    case 404:
      return 'Traženi sadržaj ne postoji.';
    case 409:
      return 'Zahtev je u sukobu sa postojećim stanjem.';
    case 413:
      return 'Fajl je prevelik.';
    case 429:
      return 'Previše zahteva. Sačekaj malo pa pokušaj ponovo.';
  }
  return err.status >= 500 ? 'Server trenutno ne može da obradi zahtev. Pokušaj kasnije.' : 'Došlo je do greške. Pokušaj ponovo.';
}
