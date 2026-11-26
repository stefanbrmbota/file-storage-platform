import { Component, OnInit, inject, signal } from '@angular/core';
import { Notification, NotificationsService } from '../../core/notifications/notifications.service';
import { apiErrorMessage } from '../../core/http/api-error';

/** Format vremena, npr. "24. nov 2026. 12:00". */
const timeFormat = new Intl.DateTimeFormat('sr-Latn-RS', { dateStyle: 'medium', timeStyle: 'short' });

/** Kratak opis tipa obavestenja (design 3.5). */
const TYPE_LABELS: Record<string, string> = {
  SHARE_GRANTED: 'Deljenje',
  SHARE_REVOKED: 'Ukinut pristup',
  QUOTA_WARNING: 'Prostor',
  SHARED_FOLDER_UPLOAD: 'Novi fajl',
};

/**
 * Obavestenja prijavljenog korisnika (US-08, T-08.2): od najnovijeg, sa
 * tekstom i vremenom, "Ucitaj jos" za sledecu stranicu. Korisnik vidi samo
 * svoja: gateway salje servisu ID iz tokena (T-08.1).
 */
@Component({
  imports: [],
  selector: 'app-obavestenja',
  styleUrl: './obavestenja.css',
  templateUrl: './obavestenja.html',
})
export class Obavestenja implements OnInit {
  private readonly notifications = inject(NotificationsService);

  protected readonly items = signal<Notification[]>([]);
  protected readonly nextCursor = signal<string | null>(null);
  protected readonly loading = signal(true);
  protected readonly loadingMore = signal(false);
  protected readonly error = signal<string | null>(null);

  ngOnInit(): void {
    this.load();
    this.notifications.refreshUnread();
  }

  /** Sledeca stranica se dodaje ispod postojecih. */
  protected loadMore(): void {
    const cursor = this.nextCursor();
    if (cursor && !this.loadingMore()) {
      this.load(cursor);
    }
  }

  protected formatTime(iso: string): string {
    const d = new Date(iso);
    return isNaN(d.getTime()) ? '' : timeFormat.format(d);
  }

  protected typeLabel(type: string): string {
    return TYPE_LABELS[type] ?? 'Obaveštenje';
  }

  private load(cursor?: string): void {
    if (cursor) {
      this.loadingMore.set(true);
    } else {
      this.loading.set(true);
    }
    this.error.set(null);

    this.notifications.list(cursor).subscribe({
      next: (page) => {
        this.items.update((prev) => (cursor ? [...prev, ...page.items] : page.items));
        this.nextCursor.set(page.nextCursor ?? null);
        this.loading.set(false);
        this.loadingMore.set(false);
      },
      error: (err) => {
        this.error.set(apiErrorMessage(err));
        this.loading.set(false);
        this.loadingMore.set(false);
      },
    });
  }
}
