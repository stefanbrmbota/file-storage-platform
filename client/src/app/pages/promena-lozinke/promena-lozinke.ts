import { Component, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { AbstractControl, FormBuilder, ReactiveFormsModule, ValidationErrors, ValidatorFn, Validators } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { TokenService } from '../../core/auth/token.service';
import { apiErrorMessage } from '../../core/http/api-error';
import { MAX_PASSWORD, passwordValidator, passwordsMatch } from '../registracija/password-rules';

/** Nova lozinka mora biti drugacija od trenutne (isto pravilo kao na serveru). */
const differsFromCurrent: ValidatorFn = (group: AbstractControl): ValidationErrors | null => {
  const current = group.get('currentPassword')?.value as string;
  const next = group.get('newPassword')?.value as string;
  return next !== '' && next === current ? { same: true } : null;
};

/**
 * Promena lozinke (US-14, T-14.4): trenutna, nova i ponovljena lozinka. Nova
 * lozinka se proverava istim pravilima kao pri registraciji. Greske servera
 * (pogresna trenutna lozinka, prerano menjanje, pravila...) prikazuju se u formi.
 *
 * Posle uspeha Users servis opoziva token kojim je zahtev poslat, pa klijent
 * brise sacuvani token i traži novu prijavu. Lozinke se nigde ne cuvaju, ne
 * loguju i ne idu u URL.
 */
@Component({
  selector: 'app-promena-lozinke',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './promena-lozinke.html',
  styleUrl: './promena-lozinke.css',
})
export class PromenaLozinke {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);
  private readonly tokens = inject(TokenService);
  private readonly fb = inject(FormBuilder);

  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  /** Posle uspeha: lozinka je promenjena i korisnik mora ponovo da se prijavi. */
  protected readonly done = signal(false);

  protected readonly form = this.fb.nonNullable.group(
    {
      currentPassword: ['', [Validators.required, Validators.maxLength(MAX_PASSWORD)]],
      newPassword: ['', [passwordValidator]],
      newPasswordConfirm: ['', [Validators.required]],
    },
    { validators: [passwordsMatch('newPassword', 'newPasswordConfirm'), differsFromCurrent] },
  );

  protected submit(): void {
    if (this.form.invalid || this.busy()) {
      this.form.markAllAsTouched();
      return;
    }
    this.busy.set(true);
    this.error.set(null);
    this.http.post<unknown>(`${this.config.gatewayUrl}/api/users/me/password`, this.form.getRawValue()).subscribe({
      next: () => {
        this.busy.set(false);
        this.form.reset(); // lozinke ne ostaju u formi
        this.tokens.clear(); // server je opozvao token zahteva
        this.done.set(true);
      },
      error: (err: unknown) => {
        this.busy.set(false);
        this.error.set(apiErrorMessage(err));
      },
    });
  }
}
