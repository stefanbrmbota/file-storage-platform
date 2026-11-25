import { Component, OnInit, computed, inject, signal } from '@angular/core';
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
}

/** Od ovog zauzeca (u %) traka postaje crvena. */
const HIGH_USAGE_PERCENT = 90;

/**
 * Profil (US-06, T-06.3): ime, email, uloga i kvota skladista: koliko je
 * dozvoljeno, koliko je zauzeto i koliko je jos preostalo. Podaci se ucitavaju
 * pri otvaranju stranice (GET /api/users/me); identitet odredjuje gateway iz
 * tokena, klijent ne salje nikakav ID.
 */
@Component({
  selector: 'app-profil',
  imports: [RouterLink],
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

  ngOnInit(): void {
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.http.get<Account>(`${this.config.gatewayUrl}/api/users/me`).subscribe({
      next: (a) => {
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
