import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';
import { passwordValidator, passwordsMatch } from '../registracija/password-rules';

type State = 'form' | 'done' | 'missing';

/**
 * Reset lozinke (US-15, T-15.5): stranica na koju vodi link iz mejla,
 * /reset-lozinke#token=... Token je u fragmentu (ne ide serveru u URL-u ni u
 * logove). Stranica ga odmah cita i uklanja iz adrese, pa ga drzi samo u memoriji
 * i salje u telu zahteva zajedno sa novom lozinkom (POST /api/auth/password-reset/confirm).
 * Nova lozinka se proverava istim pravilima kao pri registraciji. Greske servera
 * (los link, pravila, istovremena promena) prikazuju se u formi, a link za novi
 * zahtev je uvek na stranici. Lozinka i token se nigde ne cuvaju i ne loguju.
 */
@Component({
  selector: 'app-reset-lozinke',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './reset-lozinke.html',
  styleUrl: './reset-lozinke.css',
})
export class ResetLozinke implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly fb = inject(FormBuilder);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly state = signal<State>('form');
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);

  private token: string | null = null;

  protected readonly form = this.fb.nonNullable.group(
    {
      newPassword: ['', [passwordValidator]],
      newPasswordConfirm: ['', [Validators.required]],
    },
    { validators: [passwordsMatch('newPassword', 'newPasswordConfirm')] },
  );

  /**
   * Reaguje na svaku promenu fragmenta, ne samo na prvu: kad se novi link otvori
   * u kartici u kojoj je vec /reset-lozinke, Angular ne pravi stranicu iznova.
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

      this.error.set(null);
      this.form.reset();
      this.state.set(this.token ? 'form' : 'missing');
    });
  }

  protected submit(): void {
    if (!this.token || this.form.invalid || this.busy()) {
      this.form.markAllAsTouched();
      return;
    }
    const v = this.form.getRawValue();
    this.busy.set(true);
    this.error.set(null);
    this.http
      .post<unknown>(`${this.config.gatewayUrl}/api/auth/password-reset/confirm`, {
        token: this.token,
        newPassword: v.newPassword,
        newPasswordConfirm: v.newPasswordConfirm,
      })
      .subscribe({
        next: () => {
          this.busy.set(false);
          this.token = null; // jednokratan, vise ne treba
          this.form.reset(); // lozinke ne ostaju u formi
          this.state.set('done');
        },
        error: (err: unknown) => {
          this.busy.set(false);
          this.error.set(apiErrorMessage(err));
        },
      });
  }
}
