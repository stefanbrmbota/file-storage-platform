import { Component, OnInit, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { RouterOutlet } from '@angular/router';
import { ConfigService } from './core/config.service';

type GatewayStatus = 'proveravam' | 'dostupan' | 'nedostupan';

@Component({
  imports: [RouterOutlet],
  selector: 'app-root',
  styleUrl: './app.css',
  templateUrl: './app.html',
})
export class App implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  protected readonly gatewayStatus = signal<GatewayStatus>('proveravam');

  ngOnInit(): void {
    // Provera da klijent dopire do gateway-a (T-32.1).
    this.http.get<{ status: string }>(`${this.config.gatewayUrl}/health`).subscribe({
      next: (res) => this.gatewayStatus.set(res.status === 'ok' ? 'dostupan' : 'nedostupan'),
      error: () => this.gatewayStatus.set('nedostupan'),
    });
  }
}
