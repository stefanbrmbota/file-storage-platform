import { Injectable, signal } from '@angular/core';

/**
 * Poruka o gresci koja se prikazuje na vrhu svake stranice (traka u Shell-u).
 * Interceptor ovde sam prijavljuje greske servera i mreze; stranica moze da
 * prijavi i svoju (npr. show(apiErrorMessage(err))).
 */
@Injectable({ providedIn: 'root' })
export class ErrorBannerService {
  private readonly current = signal<string | null>(null);
  readonly message = this.current.asReadonly();

  show(message: string): void {
    this.current.set(message);
  }

  dismiss(): void {
    this.current.set(null);
  }
}
