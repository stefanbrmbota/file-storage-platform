import { Component, DestroyRef, effect, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { filter } from 'rxjs';
import { ErrorBannerService } from '../core/notify/error-banner.service';
import { TokenService } from '../core/auth/token.service';
import { AuthService } from '../core/auth/auth.service';
import { NotificationsService } from '../core/notifications/notifications.service';

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
  /** Nova stranica dobija stavku ovde (uputstvo: docs/klijent.md). */
  protected readonly nav: NavItem[] = [
    { label: 'Početna', path: '/' },
    { label: 'Moji fajlovi', path: '/folderi' },
    { label: 'Profil', path: '/profil' },
    { label: 'Obaveštenja', path: '/obavestenja' },
  ];

  /** Putanja stavke uz koju se prikazuje broj neprocitanih (T-08.2). */
  protected readonly notificationsPath = '/obavestenja';

  /** Poruka o gresci na vrhu stranice (T-32.3). */
  protected readonly errors = inject(ErrorBannerService);

  /** Da li je korisnik prijavljen; navigacija i "Odjavi se" samo tada (T-32.4). */
  protected readonly tokens = inject(TokenService);
  private readonly auth = inject(AuthService);
  protected readonly loggingOut = signal(false);

  /** Broj neprocitanih obavestenja za bedz (T-08.2). */
  private readonly notifications = inject(NotificationsService);
  protected readonly unread = this.notifications.unreadCount;

  constructor() {
    // Posle prijave ucitaj broj neprocitanih, posle odjave ga obrisi.
    effect(() => {
      if (this.tokens.isAuthenticated()) {
        this.notifications.refreshUnread();
      } else {
        this.notifications.reset();
      }
    });
    // Pri prelasku na drugu stranicu osvezi broj (bez stalnog proveravanja u pozadini).
    inject(Router)
      .events.pipe(
        filter((e) => e instanceof NavigationEnd),
        takeUntilDestroyed(inject(DestroyRef)),
      )
      .subscribe(() => {
        if (this.tokens.isAuthenticated()) {
          this.notifications.refreshUnread();
        }
      });
  }

  protected logout(): void {
    this.loggingOut.set(true);
    this.auth.logout().subscribe({
      complete: () => this.loggingOut.set(false),
      error: () => this.loggingOut.set(false), // token je svakako obrisan lokalno
    });
  }
}
