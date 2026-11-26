import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Admin, AdminQuotaRequest, AdminUser } from './admin';
import { ConfigService } from '../../core/config.service';
import { formatSize } from '../folderi/files-api';

const GW = 'http://gw.test';
const USERS = `${GW}/api/admin/users`;
const REQUESTS = `${GW}/api/admin/quota-requests`;
const GB = 1024 * 1024 * 1024;

function user(overrides: Partial<AdminUser> = {}): AdminUser {
  return { id: 'u1', firstName: 'Ana', lastName: 'Anić', email: 'ana@example.com', role: 'USER', active: true, quotaBytes: GB, usedBytes: 1024, ...overrides };
}

function request(overrides: Partial<AdminQuotaRequest> = {}): AdminQuotaRequest {
  return {
    id: 'r1', userId: 'u1', firstName: 'Ana', lastName: 'Anić', email: 'ana@example.com',
    currentQuotaBytes: GB, requestedBytes: 5 * GB, status: 'PENDING', createdAt: '2026-11-26T12:00:00Z', ...overrides,
  };
}

describe('Administracija (korisnici i zahtevi za kvotu)', () => {
  let http: HttpTestingController;

  function setup() {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    const fixture = TestBed.createComponent(Admin);
    fixture.detectChanges();
    return fixture;
  }

  afterEach(() => http.verify());

  const el = (f: ReturnType<typeof setup>) => f.nativeElement as HTMLElement;

  /** Odgovara na oba poziva; undefined znaci da se taj poziv ostavlja otvoren. */
  async function open(users: AdminUser[], requests: AdminQuotaRequest[]) {
    const f = setup();
    const u = http.expectOne(USERS);
    const r = http.expectOne(REQUESTS);
    expect(u.request.method).toBe('GET');
    expect(r.request.method).toBe('GET');
    u.flush({ users });
    r.flush({ requests });
    await f.whenStable();
    f.detectChanges();
    return f;
  }

  const cells = (row: Element) => Array.from(row.querySelectorAll('td')).map((c) => c.textContent?.replace(/\s+/g, ' ').trim());

  it('zove obe liste bez ikakvog ID-ja u adresi', async () => {
    const f = setup();
    expect(http.expectOne(USERS).request.urlWithParams).toBe(USERS);
    expect(http.expectOne(REQUESTS).request.urlWithParams).toBe(REQUESTS);
    expect(el(f).textContent).toContain('Učitavam');
  });

  it('tabela korisnika prikazuje ime, email, kvotu i zauzeće', async () => {
    const f = await open([user()], []);
    const row = el(f).querySelector('.admin__users tbody tr')!;
    expect(cells(row)).toEqual(['Ana Anić', 'ana@example.com', formatSize(GB), formatSize(1024)]);
  });

  it('tabela zahteva prikazuje podnosioca, trenutnu i traženu kvotu i status', async () => {
    const f = await open([], [request()]);
    const row = el(f).querySelector('.admin__requests tbody tr')!;
    const c = cells(row);
    expect(c.slice(0, 5)).toEqual(['Ana Anić', 'ana@example.com', formatSize(GB), formatSize(5 * GB), 'na čekanju']);
    expect(c[5]).toMatch(/^\d{2}\.\d{2}\.\d{4}\. \d{2}:\d{2}$/);
  });

  it('prikazuje sve statuse zahteva', async () => {
    const f = await open([], [
      request({ id: 'a', status: 'PENDING' }),
      request({ id: 'b', status: 'APPROVED' }),
      request({ id: 'c', status: 'REJECTED' }),
    ]);
    const statuses = Array.from(el(f).querySelectorAll('.admin__status')).map((s) => s.textContent?.trim());
    expect(statuses).toEqual(['na čekanju', 'odobren', 'odbijen']);
    expect(el(f).querySelector('.admin__status--pending')).not.toBeNull();
    expect(el(f).querySelector('.admin__status--approved')).not.toBeNull();
    expect(el(f).querySelector('.admin__status--rejected')).not.toBeNull();
  });

  it('podnosilac sa obrisanim nalogom se prikazuje bez praznih polja i bez trenutne kvote', async () => {
    const f = await open([], [request({ firstName: '', lastName: '', email: '', currentQuotaBytes: 0 })]);
    const c = cells(el(f).querySelector('.admin__requests tbody tr')!);
    expect(c[0]).toBe('(nalog obrisan)');
    expect(c[2]).toBe('');
  });

  it('administrator i neaktivan nalog imaju oznaku', async () => {
    const f = await open([user({ id: 'a', role: 'ADMIN' }), user({ id: 'b', active: false })], []);
    const rows = el(f).querySelectorAll('.admin__users tbody tr');
    expect(rows[0].textContent).toContain('admin');
    expect(rows[1].textContent).toContain('neaktivan');
  });

  it('prazne liste prikazuju poruku', async () => {
    const f = await open([], []);
    const t = el(f).textContent ?? '';
    expect(t).toContain('Nema zahteva za kvotu.');
    expect(t).toContain('Nema korisnika.');
    expect(el(f).querySelector('table')).toBeNull();
  });

  it('podaci sa servera se prikazuju kao tekst (bez HTML-a)', async () => {
    const f = await open([user({ firstName: '<img src=x onerror=alert(1)>', lastName: '<b>Zlo</b>' })], []);
    expect(el(f).querySelector('img')).toBeNull();
    expect(el(f).querySelector('.admin__users b')).toBeNull();
    expect(el(f).textContent).toContain('<b>Zlo</b>');
  });

  it('greška jedne liste ne skriva drugu, a ponovni pokušaj učitava samo tu listu', async () => {
    const f = setup();
    http.expectOne(USERS).flush({ users: [user()] });
    http.expectOne(REQUESTS).flush({ error: 'zabranjen pristup' }, { status: 403, statusText: 'Forbidden' });
    await f.whenStable();
    f.detectChanges();

    expect(el(f).querySelector('.admin__users')).not.toBeNull();
    expect(el(f).querySelector('[role=alert]')?.textContent).toContain('zabranjen pristup');

    el(f).querySelector<HTMLButtonElement>('button')!.click();
    http.expectOne(REQUESTS).flush({ requests: [request()] });
    await f.whenStable();
    f.detectChanges();
    expect(el(f).querySelector('.admin__requests')).not.toBeNull();
    expect(el(f).querySelector('[role=alert]')).toBeNull();
  });

  it('neispravan odgovor servera ne ruši stranicu', async () => {
    const f = setup();
    http.expectOne(USERS).flush({});
    http.expectOne(REQUESTS).flush(null);
    await f.whenStable();
    f.detectChanges();
    expect(el(f).textContent).toContain('Nema korisnika.');
    expect(el(f).textContent).toContain('Nema zahteva za kvotu.');
  });
});
