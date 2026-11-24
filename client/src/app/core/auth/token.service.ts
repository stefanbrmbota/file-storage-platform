import { Injectable, computed, signal } from '@angular/core';

/** Podaci iz JWT-a koje klijent koristi za prikaz (ne za proveru prava). */
export interface TokenClaims {
  /** ID korisnika (claim "sub"). */
  userId: string;
  /** Uloga: USER ili ADMIN. */
  role: string;
  /** Kada token istice. */
  expiresAt: Date;
}

const STORAGE_KEY = 'fsp.token';

/**
 * Cuva JWT posle prijave i brise ga pri odjavi (T-32.3).
 *
 * Token je u sessionStorage: prezivi osvezavanje stranice, a brise se kad se
 * tab zatvori, pa ostaje kraci period nego u localStorage. Istekao token se
 * odmah brise. Klijent token ne proverava kriptografski; to radi gateway.
 * Claims sluze samo za prikaz (npr. da li prikazati admin link).
 */
@Injectable({ providedIn: 'root' })
export class TokenService {
  private readonly current = signal<string | null>(this.readStored());

  /** Trenutni token ili null. */
  readonly token = computed(() => this.current());
  /** Da li je korisnik prijavljen (postoji neistekao token). */
  readonly isAuthenticated = computed(() => this.current() !== null);
  /** Podaci iz tokena za prikaz, ili null. */
  readonly claims = computed(() => {
    const t = this.current();
    return t ? decodeClaims(t) : null;
  });

  /** Vraca token ako postoji i nije istekao; istekao brise. */
  get(): string | null {
    const t = this.current();
    if (t && isExpired(t)) {
      this.clear();
      return null;
    }
    return t;
  }

  /** Cuva token posle prijave. Neispravan ili istekao token se ne cuva. */
  set(token: string): void {
    if (!decodeClaims(token) || isExpired(token)) {
      this.clear();
      return;
    }
    this.current.set(token);
    try {
      sessionStorage.setItem(STORAGE_KEY, token);
    } catch {
      // Storage moze biti blokiran (privatni prozor); token ostaje u memoriji.
    }
  }

  /** Brise token (odjava, 401). */
  clear(): void {
    this.current.set(null);
    try {
      sessionStorage.removeItem(STORAGE_KEY);
    } catch {
      // ignore
    }
  }

  private readStored(): string | null {
    let t: string | null = null;
    try {
      t = sessionStorage.getItem(STORAGE_KEY);
    } catch {
      return null;
    }
    if (!t || !decodeClaims(t) || isExpired(t)) {
      try {
        sessionStorage.removeItem(STORAGE_KEY);
      } catch {
        // ignore
      }
      return null;
    }
    return t;
  }
}

/** Cita payload JWT-a (bez provere potpisa). Vraca null za neispravan token. */
export function decodeClaims(token: string): TokenClaims | null {
  const parts = token.split('.');
  if (parts.length !== 3) {
    return null;
  }
  try {
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    const payload = JSON.parse(atob(b64.padEnd(b64.length + ((4 - (b64.length % 4)) % 4), '=')));
    if (typeof payload.sub !== 'string' || typeof payload.exp !== 'number') {
      return null;
    }
    return {
      userId: payload.sub,
      role: typeof payload.role === 'string' ? payload.role : '',
      expiresAt: new Date(payload.exp * 1000),
    };
  } catch {
    return null;
  }
}

function isExpired(token: string): boolean {
  const c = decodeClaims(token);
  return !c || c.expiresAt.getTime() <= Date.now();
}
