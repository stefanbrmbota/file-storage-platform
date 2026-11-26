import { Component, OnInit, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { RouterLink } from '@angular/router';
import { ConfigService } from '../../core/config.service';
import { TokenService } from '../../core/auth/token.service';

type GatewayStatus = 'proveravam' | 'dostupan' | 'nedostupan';

/** Pocetna stranica (zasticena). Za sada prikazuje ulogu i da li klijent dopire do gateway-a. */
@Component({
  selector: 'app-home',
  imports: [RouterLink],
  templateUrl: './home.html',
  styleUrl: './home.css',
})
export class Home implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  protected readonly gatewayStatus = signal<GatewayStatus>('proveravam');
  protected readonly claims = inject(TokenService).claims;

  ngOnInit(): void {
    this.http.get<{ status: string }>(`${this.config.gatewayUrl}/health`).subscribe({
      next: (res) => this.gatewayStatus.set(res.status === 'ok' ? 'dostupan' : 'nedostupan'),
      error: () => this.gatewayStatus.set('nedostupan'),
    });
  }
}
