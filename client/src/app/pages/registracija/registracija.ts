import { Component, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { AbstractControl, FormBuilder, ReactiveFormsModule, ValidationErrors, ValidatorFn, Validators } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';

/** Granice isto kao u Users servisu (service/registration_validation.go). */
const MAX_NAME = 50;
const MAX_EMAIL = 254;
const MIN_PASSWORD = 8;
const MAX_PASSWORD = 128;

/** Broj karaktera (kao Go utf8.RuneCountInString), ne UTF-16 jedinica. */
function length(value: string): number {
  return [...value].length;
}

/** Ime ili prezime: obavezno (bez razmaka na krajevima) i najvise 50 karaktera. */
const nameValidator: ValidatorFn = (c: AbstractControl): ValidationErrors | null => {
  const v = String(c.value ?? '').trim();
  if (v === '') {
    return { required: true };
  }
  return length(v) > MAX_NAME ? { maxlength: true } : null;
};

/** Email: obavezan, najvise 254 znaka, oblik a@b.c (domen sa tackom). */
const emailValidator: ValidatorFn = (c: AbstractControl): ValidationErrors | null => {
  const v = String(c.value ?? '').trim();
  if (v === '') {
    return { required: true };
  }
  if (v.length > MAX_EMAIL) {
    return { maxlength: true };
  }
  return /^[^@\s]+@[^@\s.][^@\s]*\.[^@\s.]+$/.test(v) ? null : { email: true };
};

/** Jaka lozinka: 8 do 128 karaktera, veliko i malo slovo, broj i specijalni znak. */
const passwordValidator: ValidatorFn = (c: AbstractControl): ValidationErrors | null => {
  const v = String(c.value ?? '');
  if (v === '') {
    return { required: true };
  }
  const errors: ValidationErrors = {};
  const n = length(v);
  if (n < MIN_PASSWORD) {
    errors['minlength'] = true;
  }
  if (n > MAX_PASSWORD) {
    errors['maxlength'] = true;
  }
  if (!/\p{Lu}/u.test(v)) {
    errors['upper'] = true;
  }
  if (!/\p{Ll}/u.test(v)) {
    errors['lower'] = true;
  }
  if (!/\p{Nd}/u.test(v)) {
    errors['digit'] = true;
  }
  if (!/[\p{P}\p{S}]/u.test(v)) {
    errors['special'] = true;
  }
  return Object.keys(errors).length > 0 ? errors : null;
};

/** Ponovljena lozinka mora biti ista kao lozinka. */
const passwordsMatch: ValidatorFn = (group: AbstractControl): ValidationErrors | null => {
  const password = group.get('password')?.value as string;
  const confirm = group.get('passwordConfirm')?.value as string;
  return confirm !== '' && confirm !== password ? { mismatch: true } : null;
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
    { validators: [passwordsMatch] },
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
