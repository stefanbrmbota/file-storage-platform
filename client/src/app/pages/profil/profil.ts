import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';
import { formatSize } from '../folderi/files-api';

/** Odgovor GET /api/users/me (US-06): nalog i kvota prijavljenog korisnika. */
export interface Account {
  id: string;
  firstName: string;
  lastName: string;
  email: string;
  role: 'USER' | 'ADMIN';
  quotaBytes: number;
  usedBytes: number;
  remainingBytes: number;
  /** Kada lozinka istice (ISO, UTC; US-16): posle toga prijava nije moguca dok se lozinka ne promeni. */
  passwordExpiresAt?: string;
}

/** Od ovog zauzeca (u %) traka postaje crvena. */
const HIGH_USAGE_PERCENT = 90;

/** U poslednjih toliko dana pre isteka lozinke prikazuje se upozorenje (US-16). */
const PASSWORD_WARN_DAYS = 7;

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** Nivo upozorenja o roku lozinke: u redu, uskoro istice, vec istekla. */
export type PasswordLevel = 'ok' | 'warn' | 'expired';

export interface PasswordStatus {
  level: PasswordLevel;
  /** Trenutak isteka (za prikaz datuma). */
  expiresAt: Date;
  /** Poruka o preostalom vremenu ili isteku. */
  text: string;
}

/** Srpski oblik reci uz broj: 1 dan, 2 dana, 5 dana, 11 dana, 21 dan. */
function plural(n: number, one: string, few: string, many: string): string {
  const last = n % 10;
  const lastTwo = n % 100;
  if (last === 1 && lastTwo !== 11) {
    return one;
  }
  if (last >= 2 && last <= 4 && !(lastTwo >= 12 && lastTwo <= 14)) {
    return few;
  }
  return many;
}

/** Preostalo vreme u najkrupnijoj jedinici: "59 dana", "5 sati", "1 minut", "manje od minut". */
export function remainingText(ms: number): string {
  if (ms >= DAY) {
    const d = Math.floor(ms / DAY);
    return `${d} ${plural(d, 'dan', 'dana', 'dana')}`;
  }
  if (ms >= HOUR) {
    const h = Math.floor(ms / HOUR);
    return `${h} ${plural(h, 'sat', 'sata', 'sati')}`;
  }
  if (ms >= MINUTE) {
    const m = Math.floor(ms / MINUTE);
    return `${m} ${plural(m, 'minut', 'minuta', 'minuta')}`;
  }
  return 'manje od minut';
}

/**
 * Profil (US-06, T-06.3): ime, email, uloga i kvota skladista: koliko je
 * dozvoljeno, koliko je zauzeto i koliko je jos preostalo. Podaci se ucitavaju
 * pri otvaranju stranice (GET /api/users/me); identitet odredjuje gateway iz
 * tokena, klijent ne salje nikakav ID.
 */
@Component({
  selector: 'app-profil',
  imports: [RouterLink, DatePipe],
  templateUrl: './profil.html',
  styleUrl: './profil.css',
})
export class Profil implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  protected readonly account = signal<Account | null>(null);
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);

  protected readonly formatSize = formatSize;

  /** Zauzece u procentima (0 do 100); kvota 0 ili manja se racuna kao 0, bez deljenja nulom. */
  protected readonly percent = computed(() => {
    const a = this.account();
    if (!a || a.quotaBytes <= 0 || a.usedBytes <= 0) {
      return 0;
    }
    return Math.min(100, (a.usedBytes / a.quotaBytes) * 100);
  });

  /** Tekst uz traku: "0%", "<0.1%" za sasvim malo zauzece, inace jedna decimala. */
  protected readonly percentLabel = computed(() => {
    const a = this.account();
    const p = this.percent();
    if (!a || a.usedBytes <= 0 || a.quotaBytes <= 0) {
      return '0%';
    }
    return p < 0.1 ? '<0.1%' : `${p.toFixed(1)}%`;
  });

  protected readonly high = computed(() => this.percent() >= HIGH_USAGE_PERCENT);

  protected readonly roleLabel = computed(() => (this.account()?.role === 'ADMIN' ? 'Administrator' : 'Korisnik'));

  /** Trenutak ucitavanja podataka; od njega se racuna koliko je jos preostalo do isteka lozinke. */
  private readonly loadedAt = signal(Date.now());

  /**
   * Rok lozinke (US-16). Ako server ne posalje rok ili je neispravan, nema prikaza
   * (stranica ostaje upotrebljiva). Upozorenje je u poslednjih 7 dana, a posle isteka
   * (token izdat pre isteka jos vazi) korisnik se poziva da lozinku odmah promeni.
   */
  protected readonly password = computed<PasswordStatus | null>(() => {
    const raw = this.account()?.passwordExpiresAt;
    const at = raw ? Date.parse(raw) : NaN;
    if (Number.isNaN(at)) {
      return null;
    }
    const left = at - this.loadedAt();
    const expiresAt = new Date(at);
    if (left <= 0) {
      return {
        level: 'expired',
        expiresAt,
        text: 'Lozinka je istekla. Promeni je odmah: posle odjave prijava neće biti moguća dok je ne postaviš preko opcije „Zaboravio si lozinku?“.',
      };
    }
    const level: PasswordLevel = left <= PASSWORD_WARN_DAYS * DAY ? 'warn' : 'ok';
    return { level, expiresAt, text: `Još ${remainingText(left)}.` };
  });

  ngOnInit(): void {
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.http.get<Account>(`${this.config.gatewayUrl}/api/users/me`).subscribe({
      next: (a) => {
        this.loadedAt.set(Date.now());
        this.account.set(a);
        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.account.set(null);
        this.error.set(apiErrorMessage(err));
        this.loading.set(false);
      },
    });
  }
}
