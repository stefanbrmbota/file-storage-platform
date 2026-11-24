import { Component, OnInit, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';

/**
 * PRIMER zasticene stranice (US-32, T-32.5) za kopiranje: kako se zove
 * gateway, prikazuje ucitavanje i greska. Uputstvo: docs/klijent.md.
 *
 * Ruta /primer (zasticena, nije u navigaciji). Poziva GET /api/auth/session,
 * zasticenu rutu koja postoji, samo da pokaze obrazac. Token NE dodajes
 * rucno: interceptor ga dodaje sam, a na 401 vodi na prijavu.
 */
@Component({
  imports: [],
  selector: 'app-primer',
  styleUrl: './primer.css',
  templateUrl: './primer.html',
})
export class Primer implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly data = signal<{ userId: string; role: string } | null>(null);

  ngOnInit(): void {
    // Adresa uvek ide preko config.gatewayUrl, nikad zakucana.
    this.http.get<{ userId: string; role: string }>(`${this.config.gatewayUrl}/api/auth/session`).subscribe({
      next: (res) => {
        this.data.set(res);
        this.loading.set(false);
      },
      error: (err) => {
        // Poruka iz {"error": "..."} ili opsta poruka za status.
        this.error.set(apiErrorMessage(err));
        this.loading.set(false);
      },
    });
  }
}
