import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { TokenService } from '../../core/auth/token.service';
import { apiErrorMessage } from '../../core/http/api-error';

type State = 'loading' | 'error' | 'missing';

/**
 * Prijava magicnim linkom (US-17, T-17.5): stranica na koju vodi link iz mejla,
 * /magicni-link#token=... Token je u fragmentu (ne ide serveru u URL-u ni u logove).
 * Stranica ga odmah cita i uklanja iz adrese, pa salje POST /api/auth/magic-link/login
 * sa tokenom u telu. Uspeh vraca JWT koji se cuva kao posle obicne prijave, a
 * korisnik ide na pocetnu. Link je jednokratan: drugi klik na isti link daje gresku
 * sa vezom za novi zahtev. Token ostaje samo u memoriji i nikad se ne loguje.
 */
@Component({
  selector: 'app-magicni-link',
  imports: [RouterLink],
  templateUrl: './magicni-link.html',
  styleUrl: './magicni-link.css',
})
export class MagicniLink implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);
  private readonly tokens = inject(TokenService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly state = signal<State>('loading');
  protected readonly error = signal<string | null>(null);
  /** Moze se ponoviti samo kad je servis privremeno nedostupan (502): token je tada najverovatnije jos vazeci. */
  protected readonly canRetry = signal(false);

  private token: string | null = null;

  /**
   * Reaguje na svaku promenu fragmenta, ne samo na prvu: kad se novi link otvori
   * u kartici u kojoj je vec /magicni-link, Angular ne pravi stranicu iznova.
   * Prazan fragment posle prve provere ignorise (to je adresa koju smo sami ocistili).
   */
  ngOnInit(): void {
    let first = true;
    this.route.fragment.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((fragment) => {
      const isFirst = first;
      first = false;
      if (fragment === null && !isFirst) {
        return;
      }
      const f = fragment ?? '';
      this.token = f.startsWith('token=') ? f.slice('token='.length) : null;

      // Token ne treba da ostane u adresi (istorija, deljenje ekrana).
      if (fragment !== null) {
        void this.router.navigate([], { relativeTo: this.route, replaceUrl: true });
      }

      if (!this.token) {
        this.state.set('missing');
        return;
      }
      this.login();
    });
  }

  protected login(): void {
    if (!this.token) {
      return;
    }
    this.state.set('loading');
    this.error.set(null);
    this.canRetry.set(false);
    this.http.post<{ token?: unknown }>(`${this.config.gatewayUrl}/api/auth/magic-link/login`, { token: this.token }).subscribe({
      next: (res) => {
        this.token = null; // jednokratan, vise ne treba
        if (typeof res?.token === 'string') {
          this.tokens.set(res.token);
        }
        if (!this.tokens.get()) {
          this.fail('Prijava nije uspela: odgovor servera nije ispravan.', false);
          return;
        }
        void this.router.navigateByUrl('/');
      },
      error: (err: unknown) => this.fail(apiErrorMessage(err), err instanceof HttpErrorResponse && err.status === 502),
    });
  }

  private fail(message: string, retry: boolean): void {
    this.error.set(message);
    this.canRetry.set(retry);
    this.state.set('error');
  }
}
