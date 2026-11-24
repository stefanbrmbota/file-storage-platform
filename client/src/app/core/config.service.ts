import { Injectable } from '@angular/core';

/** Podesavanja klijenta koja se citaju pri pokretanju iz /config.json. */
export interface AppConfig {
  /** Adresa API gateway-a, npr. http://localhost:8080 (bez / na kraju). */
  gatewayUrl: string;
}

/**
 * Cita /config.json pre pokretanja aplikacije (T-32.1).
 *
 * Adresa gateway-a nije zakucana u kodu: u Docker-u config.json pravi
 * kontejner pri pokretanju iz env promenljive GATEWAY_URL, a za `ng serve`
 * se koristi public/config.json.
 */
@Injectable({ providedIn: 'root' })
export class ConfigService {
  private config: AppConfig | null = null;

  async load(): Promise<void> {
    const res = await fetch('/config.json', { cache: 'no-store' });
    if (!res.ok) {
      throw new Error(`config.json nije ucitan (status ${res.status})`);
    }
    const data = (await res.json()) as Partial<AppConfig>;
    if (typeof data.gatewayUrl !== 'string' || data.gatewayUrl.trim() === '') {
      throw new Error('config.json nema gatewayUrl');
    }
    this.config = { gatewayUrl: data.gatewayUrl.trim().replace(/\/+$/, '') };
  }

  /** Adresa gateway-a; dostupna posle load(). */
  get gatewayUrl(): string {
    if (!this.config) {
      throw new Error('konfiguracija jos nije ucitana');
    }
    return this.config.gatewayUrl;
  }

  /** Samo za testove. */
  setForTest(config: AppConfig): void {
    this.config = config;
  }
}
