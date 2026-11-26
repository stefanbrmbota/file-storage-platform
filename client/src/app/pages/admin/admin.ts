import { Component, OnInit, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';
import { formatSize } from '../folderi/files-api';

/** Stavka iz GET /api/admin/users (US-22). */
export interface AdminUser {
  id: string;
  firstName: string;
  lastName: string;
  email: string;
  role: 'USER' | 'ADMIN';
  active: boolean;
  quotaBytes: number;
  usedBytes: number;
}

/** Stavka iz GET /api/admin/quota-requests (US-22). */
export interface AdminQuotaRequest {
  id: string;
  userId: string;
  firstName: string;
  lastName: string;
  email: string;
  currentQuotaBytes: number;
  requestedBytes: number;
  status: 'PENDING' | 'APPROVED' | 'REJECTED';
  createdAt: string;
  decidedAt?: string;
}

const STATUS_LABEL: Record<AdminQuotaRequest['status'], string> = {
  PENDING: 'na čekanju',
  APPROVED: 'odobren',
  REJECTED: 'odbijen',
};

/**
 * Administracija (US-22, T-22.4): lista korisnika (ime, email, kvota, zauzece) i
 * lista zahteva za kvotu (podnosilac, trenutna i trazena kvota, status). Samo za
 * administratora (adminGuard); pravo stvarno proveravaju gateway i Users servis.
 * Svaka lista se ucitava i ponavlja nezavisno, pa greska jedne ne skriva drugu.
 */
@Component({
  selector: 'app-admin',
  imports: [DatePipe],
  templateUrl: './admin.html',
  styleUrl: './admin.css',
})
export class Admin implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  protected readonly users = signal<AdminUser[]>([]);
  protected readonly usersLoading = signal(true);
  protected readonly usersError = signal<string | null>(null);

  protected readonly requests = signal<AdminQuotaRequest[]>([]);
  protected readonly requestsLoading = signal(true);
  protected readonly requestsError = signal<string | null>(null);

  protected readonly formatSize = formatSize;
  protected readonly statusLabel = (s: AdminQuotaRequest['status']) => STATUS_LABEL[s];

  ngOnInit(): void {
    this.loadUsers();
    this.loadRequests();
  }

  protected loadUsers(): void {
    this.usersLoading.set(true);
    this.usersError.set(null);
    this.http.get<{ users: AdminUser[] }>(`${this.config.gatewayUrl}/api/admin/users`).subscribe({
      next: (res) => {
        this.users.set(Array.isArray(res?.users) ? res.users : []);
        this.usersLoading.set(false);
      },
      error: (err: unknown) => {
        this.users.set([]);
        this.usersError.set(apiErrorMessage(err));
        this.usersLoading.set(false);
      },
    });
  }

  protected loadRequests(): void {
    this.requestsLoading.set(true);
    this.requestsError.set(null);
    this.http.get<{ requests: AdminQuotaRequest[] }>(`${this.config.gatewayUrl}/api/admin/quota-requests`).subscribe({
      next: (res) => {
        this.requests.set(Array.isArray(res?.requests) ? res.requests : []);
        this.requestsLoading.set(false);
      },
      error: (err: unknown) => {
        this.requests.set([]);
        this.requestsError.set(apiErrorMessage(err));
        this.requestsLoading.set(false);
      },
    });
  }

  /** Ime podnosioca; ako je nalog obrisan, server salje prazna polja. */
  protected requester(r: AdminQuotaRequest): string {
    const name = `${r.firstName} ${r.lastName}`.trim();
    return name === '' ? '(nalog obrisan)' : name;
  }
}
