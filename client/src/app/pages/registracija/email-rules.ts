import { AbstractControl, ValidationErrors, ValidatorFn } from '@angular/forms';

/**
 * Pravilo za email, isto kao u Users servisu (service/registration_validation.go).
 * Koriste ga registracija (US-04) i zaboravljena lozinka (US-15).
 */
export const MAX_EMAIL = 254;

/** Email: obavezan, najvise 254 znaka, oblik a@b.c (domen sa tackom). */
export const emailValidator: ValidatorFn = (c: AbstractControl): ValidationErrors | null => {
  const v = String(c.value ?? '').trim();
  if (v === '') {
    return { required: true };
  }
  if (v.length > MAX_EMAIL) {
    return { maxlength: true };
  }
  return /^[^@\s]+@[^@\s.][^@\s]*\.[^@\s.]+$/.test(v) ? null : { email: true };
};
