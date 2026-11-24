import { Component, OnInit, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { ConfigService } from '../../core/config.service';

type GatewayStatus = 'proveravam' | 'dostupan' | 'nedostupan';

/** Pocetna stranica. Za sada prikazuje i da li klijent dopire do gateway-a (T-32.1). */
@Component({
  selector: 'app-home',
  templateUrl: './home.html',
  styleUrl: './home.css',
})
export class Home implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  protected readonly gatewayStatus = signal<GatewayStatus>('proveravam');

  ngOnInit(): void {
    this.http.get<{ status: string }>(`${this.config.gatewayUrl}/health`).subscribe({
      next: (res) => this.gatewayStatus.set(res.status === 'ok' ? 'dostupan' : 'nedostupan'),
      error: () => this.gatewayStatus.set('nedostupan'),
    });
  }
}
