import { Component, inject, signal } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { ErrorBannerService } from '../core/notify/error-banner.service';
import { TokenService } from '../core/auth/token.service';
import { AuthService } from '../core/auth/auth.service';

/** Stavka glavne navigacije u zaglavlju. */
export interface NavItem {
  label: string;
  path: string;
}

/**
 * Zajednicki raspored svih stranica (T-32.2): zaglavlje sa navigacijom i
 * mesto za sadrzaj stranice (<router-outlet>). Stranice se dodaju kao
 * child rute ovog rasporeda u app.routes.ts.
 */
@Component({
  selector: 'app-shell',
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  templateUrl: './shell.html',
  styleUrl: './shell.css',
})
export class Shell {
  /** Nova stranica dobija stavku ovde (vidi uputstvo u docs/, T-32.5). */
  protected readonly nav: NavItem[] = [{ label: 'Početna', path: '/' }];

  /** Poruka o gresci na vrhu stranice (T-32.3). */
  protected readonly errors = inject(ErrorBannerService);

  /** Da li je korisnik prijavljen; navigacija i "Odjavi se" samo tada (T-32.4). */
  protected readonly tokens = inject(TokenService);
  private readonly auth = inject(AuthService);
  protected readonly loggingOut = signal(false);

  protected logout(): void {
    this.loggingOut.set(true);
    this.auth.logout().subscribe({
      complete: () => this.loggingOut.set(false),
      error: () => this.loggingOut.set(false), // token je svakako obrisan lokalno
    });
  }
}
