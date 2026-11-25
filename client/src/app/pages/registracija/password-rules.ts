import { AbstractControl, ValidationErrors, ValidatorFn } from '@angular/forms';

/**
 * Pravila lozinke, ista kao u Users servisu (service/registration_validation.go).
 * Koriste ih registracija (US-04) i promena lozinke (US-14).
 */
export const MIN_PASSWORD = 8;
export const MAX_PASSWORD = 128;

/** Broj karaktera (kao Go utf8.RuneCountInString), ne UTF-16 jedinica. */
export function charCount(value: string): number {
  return [...value].length;
}

/** Jaka lozinka: 8 do 128 karaktera, veliko i malo slovo, broj i specijalni znak. */
export const passwordValidator: ValidatorFn = (c: AbstractControl): ValidationErrors | null => {
  const v = String(c.value ?? '');
  if (v === '') {
    return { required: true };
  }
  const errors: ValidationErrors = {};
  const n = charCount(v);
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

/** Ponovljena lozinka (polje confirmKey) mora biti ista kao lozinka (polje passwordKey). */
export function passwordsMatch(passwordKey: string, confirmKey: string): ValidatorFn {
  return (group: AbstractControl): ValidationErrors | null => {
    const password = group.get(passwordKey)?.value as string;
    const confirm = group.get(confirmKey)?.value as string;
    return confirm !== '' && confirm !== password ? { mismatch: true } : null;
  };
}
