import { Component, OnInit, computed, inject, input, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { ConfigService } from '../../core/config.service';
import { apiErrorMessage } from '../../core/http/api-error';
import { formatSize } from '../folderi/files-api';

/** Odgovor GET/POST /api/users/me/quota-requests (US-21). */
export interface QuotaRequest {
  id: string;
  requestedBytes: number;
  status: 'PENDING' | 'APPROVED' | 'REJECTED';
  createdAt: string;
  decidedAt?: string;
}

const MB = 1024 * 1024;
const GB = 1024 * MB;
/** Isto kao na serveru: najvise 1 PiB. */
const MAX_REQUEST_BYTES = 2 ** 50;

const STATUS_LABEL: Record<QuotaRequest['status'], string> = {
  PENDING: 'na čekanju',
  APPROVED: 'odobren',
  REJECTED: 'odbijen',
};

/**
 * Zahtev za povecanje kvote na profilu (US-21, T-21.4): korisnik unosi zeljenu
 * kvotu (MB ili GB, vecu od trenutne) i vidi status svog poslednjeg zahteva.
 * Dok je zahtev na cekanju, novi se ne moze podneti. Identitet odredjuje gateway
 * iz tokena, klijent ne salje nikakav ID.
 */
@Component({
  selector: 'app-zahtev-kvote',
  imports: [DatePipe],
  templateUrl: './zahtev-kvote.html',
  styleUrl: './zahtev-kvote.css',
})
export class ZahtevKvote implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  /** Trenutna kvota korisnika u bajtovima (sa profila). */
  readonly quotaBytes = input.required<number>();

  protected readonly loading = signal(true);
  protected readonly loadError = signal<string | null>(null);
  protected readonly latest = signal<QuotaRequest | null>(null);

  protected readonly amount = signal('');
  protected readonly unit = signal<number>(GB);
  protected readonly busy = signal(false);
  protected readonly submitError = signal<string | null>(null);
  protected readonly submitted = signal(false);

  protected readonly formatSize = formatSize;
  protected readonly statusLabel = (s: QuotaRequest['status']) => STATUS_LABEL[s];
  protected readonly MB = MB;
  protected readonly GB = GB;

  /** Dok je zahtev na cekanju, forma se ne prikazuje. */
  protected readonly pending = computed(() => this.latest()?.status === 'PENDING');

  /** Trazena kvota u bajtovima ili null ako unos nije pozitivan broj. */
  private readonly requestedBytes = computed<number | null>(() => {
    const text = this.amount().trim().replace(',', '.');
    if (text === '' || !/^\d+(\.\d+)?$/.test(text)) {
      return null;
    }
    const bytes = Math.round(Number(text) * this.unit());
    return bytes > 0 ? bytes : null;
  });

  /** Poruka o neispravnom unosu ili null kad je unos u redu. */
  protected readonly problem = computed<string | null>(() => {
    const bytes = this.requestedBytes();
    if (bytes === null) {
      return 'Unesi željenu kvotu kao pozitivan broj.';
    }
    if (bytes <= this.quotaBytes()) {
      return `Željena kvota mora biti veća od trenutne (${formatSize(this.quotaBytes())}).`;
    }
    if (bytes > MAX_REQUEST_BYTES) {
      return 'Željena kvota je prevelika.';
    }
    return null;
  });

  ngOnInit(): void {
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.http.get<QuotaRequest>(`${this.config.gatewayUrl}/api/users/me/quota-requests/latest`).subscribe({
      next: (r) => {
        this.latest.set(r);
        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.loading.set(false);
        if (err instanceof HttpErrorResponse && err.status === 404) {
          this.latest.set(null); // nema podnetog zahteva
          return;
        }
        this.loadError.set(apiErrorMessage(err));
      },
    });
  }

  protected onAmount(event: Event): void {
    this.amount.set((event.target as HTMLInputElement).value);
  }

  protected onUnit(event: Event): void {
    this.unit.set(Number((event.target as HTMLSelectElement).value));
  }

  protected submit(): void {
    this.submitted.set(true);
    const bytes = this.requestedBytes();
    if (this.busy() || this.problem() !== null || bytes === null) {
      return;
    }
    this.busy.set(true);
    this.submitError.set(null);
    this.http.post<QuotaRequest>(`${this.config.gatewayUrl}/api/users/me/quota-requests`, { requestedBytes: bytes }).subscribe({
      next: (r) => {
        this.busy.set(false);
        this.latest.set(r);
        this.amount.set('');
        this.submitted.set(false);
      },
      error: (err: unknown) => {
        this.busy.set(false);
        this.submitError.set(apiErrorMessage(err));
        if (err instanceof HttpErrorResponse && err.status === 409) {
          this.load(); // zahtev na cekanju vec postoji (npr. iz drugog taba): prikazi ga
        }
      },
    });
  }
}
