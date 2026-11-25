import { Component, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { AuthService } from '../../core/auth/auth.service';
import { apiErrorMessage } from '../../core/http/api-error';

type Step = 'credentials' | 'code';

/**
 * Prijava u dva koraka (US-32, T-32.4): email i lozinka, pa kod iz mejla.
 * Posle prijave korisnik ide na returnUrl (ako je unutrasnja adresa) ili na pocetnu.
 */
@Component({
  selector: 'app-login',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './login.html',
  styleUrl: './login.css',
})
export class Login {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly fb = inject(FormBuilder);

  protected readonly step = signal<Step>('credentials');
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly info = signal<string | null>(null);

  protected readonly credentials = this.fb.nonNullable.group({
    email: ['', [Validators.required, Validators.email, Validators.maxLength(254)]],
    password: ['', [Validators.required, Validators.maxLength(128)]],
  });

  protected readonly codeForm = this.fb.nonNullable.group({
    code: ['', [Validators.required, Validators.pattern(/^\d{6}$/)]],
  });

  /** Email iz prvog koraka (lozinka se posle prvog koraka brise iz forme). */
  protected readonly email = signal('');

  protected submitCredentials(): void {
    if (this.credentials.invalid || this.busy()) {
      this.credentials.markAllAsTouched();
      return;
    }
    const { email, password } = this.credentials.getRawValue();
    this.start();
    this.auth.login(email.trim(), password).subscribe({
      next: () => {
        this.email.set(email.trim());
        this.credentials.controls.password.reset();
        this.codeForm.reset();
        this.step.set('code');
        this.info.set(`Kod za prijavu je poslat na ${email.trim()}.`);
        this.busy.set(false);
      },
      error: (err) => this.fail(err),
    });
  }

  protected submitCode(): void {
    if (this.codeForm.invalid || this.busy()) {
      this.codeForm.markAllAsTouched();
      return;
    }
    this.start();
    this.auth.verify(this.email(), this.codeForm.getRawValue().code).subscribe({
      next: () => {
        this.busy.set(false);
        void this.router.navigateByUrl(this.safeReturnUrl());
      },
      error: (err) => this.fail(err),
    });
  }

  /** Nazad na prvi korak (pogresan email ili kod nije stigao). */
  protected back(): void {
    this.step.set('credentials');
    this.error.set(null);
    this.info.set(null);
    this.codeForm.reset();
  }

  private start(): void {
    this.busy.set(true);
    this.error.set(null);
  }

  private fail(err: unknown): void {
    this.busy.set(false);
    this.error.set(apiErrorMessage(err));
  }

  /** Samo unutrasnja adresa ("/nesto"), da link sa returnUrl ne vodi na tudji sajt. */
  private safeReturnUrl(): string {
    const url = this.route.snapshot.queryParamMap.get('returnUrl');
    if (url && url.startsWith('/') && !url.startsWith('//') && !url.startsWith('/\\') && !url.startsWith('/prijava')) {
      return url;
    }
    return '/';
  }
}
