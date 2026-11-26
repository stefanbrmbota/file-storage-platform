import { Component, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { AbstractControl, FormBuilder, ReactiveFormsModule, ValidationErrors, ValidatorFn, Validators } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';
import { emailValidator } from './email-rules';
import { charCount, passwordValidator, passwordsMatch } from './password-rules';

/** Granice isto kao u Users servisu (service/registration_validation.go). */
const MAX_NAME = 50;

/** Ime ili prezime: obavezno (bez razmaka na krajevima) i najvise 50 karaktera. */
const nameValidator: ValidatorFn = (c: AbstractControl): ValidationErrors | null => {
  const v = String(c.value ?? '').trim();
  if (v === '') {
    return { required: true };
  }
  return charCount(v) > MAX_NAME ? { maxlength: true } : null;
};

/**
 * Registracija (US-04, T-04.5): ime, prezime, email, lozinka i ponovljena
 * lozinka. Pravila se proveravaju i na klijentu (isto kao na serveru), a
 * greske servera (400, 409...) prikazuju se u formi. Posle uspeha prikazuje se
 * poruka "Proverite mejl"; nalog se aktivira linkom iz mejla (stranica /potvrda).
 * Lozinka se nigde ne cuva, ne loguje i ne ide u URL.
 */
@Component({
  selector: 'app-registracija',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './registracija.html',
  styleUrl: './registracija.css',
})
export class Registracija {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);
  private readonly fb = inject(FormBuilder);

  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  /** Email na koji je poslat link; kad je postavljen, prikazuje se poruka "Proverite mejl". */
  protected readonly sentTo = signal<string | null>(null);

  protected readonly form = this.fb.nonNullable.group(
    {
      firstName: ['', [nameValidator]],
      lastName: ['', [nameValidator]],
      email: ['', [emailValidator]],
      password: ['', [passwordValidator]],
      passwordConfirm: ['', [Validators.required]],
    },
    { validators: [passwordsMatch('password', 'passwordConfirm')] },
  );

  protected submit(): void {
    if (this.form.invalid || this.busy()) {
      this.form.markAllAsTouched();
      return;
    }
    const v = this.form.getRawValue();
    const body = {
      firstName: v.firstName.trim(),
      lastName: v.lastName.trim(),
      email: v.email.trim(),
      password: v.password,
      passwordConfirm: v.passwordConfirm,
    };
    this.busy.set(true);
    this.error.set(null);
    this.http.post<unknown>(`${this.config.gatewayUrl}/api/auth/register`, body).subscribe({
      next: () => {
        this.busy.set(false);
        this.form.reset(); // lozinka ne ostaje u formi
        this.sentTo.set(body.email);
      },
      error: (err: unknown) => {
        this.busy.set(false);
        this.error.set(apiErrorMessage(err));
      },
    });
  }
}
