import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';

type State = 'loading' | 'done' | 'error' | 'missing';

/**
 * Potvrda naloga (US-04, T-04.5): stranica na koju vodi link iz mejla,
 * /potvrda#token=... Token je u fragmentu (ne ide serveru u URL-u ni u logove).
 * Stranica ga odmah cita i uklanja iz adrese, pa salje POST /api/auth/confirm sa
 * tokenom u telu. Token ostaje samo u memoriji i nikad se ne loguje.
 */
@Component({
  selector: 'app-potvrda',
  imports: [RouterLink],
  templateUrl: './potvrda.html',
  styleUrl: './potvrda.css',
})
export class Potvrda implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly state = signal<State>('loading');
  protected readonly error = signal<string | null>(null);
  /** Moze se ponoviti samo kad je servis privremeno nedostupan (502): token je tada jos vazeci. */
  protected readonly canRetry = signal(false);

  private token: string | null = null;

  /**
   * Reaguje na svaku promenu fragmenta, ne samo na prvu: kad se novi link otvori
   * u kartici u kojoj je vec /potvrda, Angular ne pravi stranicu iznova.
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
      this.confirm();
    });
  }

  protected confirm(): void {
    if (!this.token) {
      return;
    }
    this.state.set('loading');
    this.error.set(null);
    this.canRetry.set(false);
    this.http.post<unknown>(`${this.config.gatewayUrl}/api/auth/confirm`, { token: this.token }).subscribe({
      next: () => {
        this.token = null; // jednokratan, vise ne treba
        this.state.set('done');
      },
      error: (err: unknown) => {
        this.error.set(apiErrorMessage(err));
        this.canRetry.set(err instanceof HttpErrorResponse && err.status === 502);
        this.state.set('error');
      },
    });
  }
}
