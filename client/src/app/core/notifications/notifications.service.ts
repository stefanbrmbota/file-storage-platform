import { Injectable, inject, signal } from '@angular/core';
import { HttpClient, HttpContext, HttpParams } from '@angular/common/http';
import { Observable, tap } from 'rxjs';
import { ConfigService } from '../config.service';
import { SILENT_ERRORS } from '../http/auth.interceptor';

/** Jedno obavestenje, kako ga vraca Notifications servis (GET /api/notifications). */
export interface Notification {
  id: string;
  type: string;
  message: string;
  actorId?: string;
  resourceId?: string;
  resourceType?: string;
  isRead: boolean;
  /** ISO vreme nastanka, npr. 2026-11-24T12:00:00Z. */
  createdAt: string;
}

/** Stranica obavestenja; nextCursor nema na poslednjoj stranici. */
export interface NotificationPage {
  items: Notification[];
  nextCursor?: string;
}

/**
 * Obavestenja prijavljenog korisnika (US-08, T-08.2) preko gateway-a.
 * ID korisnika se ne salje: gateway ga postavlja iz tokena (X-User-ID).
 * Broj neprocitanih je signal koji prikazuje bedz u zaglavlju.
 */
@Injectable({ providedIn: 'root' })
export class NotificationsService {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  private readonly unread = signal(0);
  /** Broj neprocitanih obavestenja (za bedz u zaglavlju). */
  readonly unreadCount = this.unread.asReadonly();

  /** Stranica obavestenja od najnovijeg; cursor je nextCursor prethodne stranice. */
  list(cursor?: string, limit = 20): Observable<NotificationPage> {
    let params = new HttpParams().set('limit', limit);
    if (cursor) {
      params = params.set('cursor', cursor);
    }
    return this.http.get<NotificationPage>(`${this.config.gatewayUrl}/api/notifications`, { params });
  }

  /**
   * Osvezava broj neprocitanih. Greska se ne prikazuje u traci (SILENT_ERRORS):
   * bedz nije vredan poruke na svakoj stranici; broj tada ostaje kakav je bio.
   */
  refreshUnread(): void {
    this.http
      .get<{ count: number }>(`${this.config.gatewayUrl}/api/notifications/unread-count`, {
        context: new HttpContext().set(SILENT_ERRORS, true),
      })
      .pipe(tap((res) => this.unread.set(typeof res?.count === 'number' ? res.count : 0)))
      .subscribe({ error: () => undefined });
  }

  /** Posle odjave bedz se brise. */
  reset(): void {
    this.unread.set(0);
  }
}
