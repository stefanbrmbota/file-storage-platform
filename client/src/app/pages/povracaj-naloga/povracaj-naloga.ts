import { Component, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, ReactiveFormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';
import { emailValidator } from '../registracija/email-rules';

/** Poruka kad server ne vrati svoju (odgovor 202 je isti za svaki ispravan zahtev). */
const DEFAULT_MESSAGE = 'Ako nalog sa tom adresom postoji, poslali smo link za prijavu.';

/**
 * Povracaj naloga magicnim linkom (US-17, T-17.5): korisnik unosi email, a server
 * (ako nalog postoji i aktivan je) salje mejl sa kratkotrajnim linkom koji ga
 * prijavljuje bez lozinke. Odgovor je uvek isti, pa stranica ne moze da otkrije
 * da li nalog postoji; zato i ona prikazuje istu opstu poruku. Link iz mejla vodi
 * na /magicni-link.
 */
@Component({
  selector: 'app-povracaj-naloga',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './povracaj-naloga.html',
  styleUrl: './povracaj-naloga.css',
})
export class PovracajNaloga {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);
  private readonly fb = inject(FormBuilder);

  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  /** Poruka servera posle uspesnog zahteva; kad je postavljena, prikazuje se "Proverite mejl". */
  protected readonly sentMessage = signal<string | null>(null);

  protected readonly form = this.fb.nonNullable.group({
    email: ['', [emailValidator]],
  });

  protected submit(): void {
    if (this.form.invalid || this.busy()) {
      this.form.markAllAsTouched();
      return;
    }
    const email = this.form.getRawValue().email.trim();
    this.busy.set(true);
    this.error.set(null);
    this.http.post<{ message?: unknown }>(`${this.config.gatewayUrl}/api/auth/magic-link/request`, { email }).subscribe({
      next: (res) => {
        this.busy.set(false);
        this.form.reset();
        this.sentMessage.set(typeof res?.message === 'string' && res.message !== '' ? res.message : DEFAULT_MESSAGE);
      },
      error: (err: unknown) => {
        this.busy.set(false);
        this.error.set(apiErrorMessage(err));
      },
    });
  }
}
