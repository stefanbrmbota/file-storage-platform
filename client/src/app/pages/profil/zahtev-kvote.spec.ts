import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { QuotaRequest, ZahtevKvote } from './zahtev-kvote';
import { ConfigService } from '../../core/config.service';
import { formatSize } from '../folderi/files-api';

const GW = 'http://gw.test';
const LATEST = `${GW}/api/users/me/quota-requests/latest`;
const SUBMIT = `${GW}/api/users/me/quota-requests`;
const MB = 1024 * 1024;
const GB = 1024 * MB;

function request(overrides: Partial<QuotaRequest> = {}): QuotaRequest {
  return { id: 'r1', requestedBytes: 5 * GB, status: 'PENDING', createdAt: '2026-11-26T12:00:00Z', ...overrides };
}

describe('Zahtev za povećanje kvote (profil)', () => {
  let http: HttpTestingController;

  function setup(quotaBytes = GB) {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    const fixture = TestBed.createComponent(ZahtevKvote);
    fixture.componentRef.setInput('quotaBytes', quotaBytes);
    fixture.detectChanges();
    return fixture;
  }

  afterEach(() => http.verify());

  const el = (f: ReturnType<typeof setup>) => f.nativeElement as HTMLElement;

  function text(f: ReturnType<typeof setup>): string {
    f.detectChanges();
    return el(f).textContent ?? '';
  }

  /** Otvara komponentu i odgovara na ucitavanje statusa (null = nema podnetog zahteva). */
  async function open(latest: QuotaRequest | null, quotaBytes = GB) {
    const f = setup(quotaBytes);
    const req = http.expectOne(LATEST);
    expect(req.request.method).toBe('GET');
    if (latest) {
      req.flush(latest);
    } else {
      req.flush({ error: 'nema podnetog zahteva za kvotu' }, { status: 404, statusText: 'Not Found' });
    }
    await f.whenStable();
    f.detectChanges();
    return f;
  }

  function fill(f: ReturnType<typeof setup>, value: string, unit?: number) {
    const input = el(f).querySelector<HTMLInputElement>('input')!;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    if (unit !== undefined) {
      const select = el(f).querySelector<HTMLSelectElement>('select')!;
      select.value = String(unit);
      select.dispatchEvent(new Event('change'));
    }
    f.detectChanges();
  }

  async function submit(f: ReturnType<typeof setup>) {
    el(f).querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    f.detectChanges();
    await f.whenStable();
  }

  it('bez podnetog zahteva prikazuje formu, bez statusa', async () => {
    const f = await open(null);
    expect(el(f).querySelector('form')).not.toBeNull();
    expect(el(f).querySelector('.zk__status')).toBeNull();
  });

  it('zahtev na čekanju prikazuje status i sakriva formu', async () => {
    const f = await open(request());
    const t = text(f);
    expect(t).toContain(formatSize(5 * GB));
    expect(t).toContain('na čekanju');
    expect(el(f).querySelector('form')).toBeNull();
    expect(t).toContain('ne može podneti');
  });

  it('odobren i odbijen zahtev prikazuju status, a forma za novi ostaje', async () => {
    for (const [status, label] of [
      ['APPROVED', 'odobren'],
      ['REJECTED', 'odbijen'],
    ] as const) {
      TestBed.resetTestingModule();
      const f = await open(request({ status }));
      expect(text(f)).toContain(label);
      expect(el(f).querySelector('form')).not.toBeNull();
      http.verify();
    }
  });

  it('šalje željenu kvotu u bajtovima (GB) i prikazuje status "na čekanju"', async () => {
    const f = await open(null);
    fill(f, '5');
    await submit(f);

    const req = http.expectOne(SUBMIT);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ requestedBytes: 5 * GB });
    expect(req.request.urlWithParams).toBe(SUBMIT);
    req.flush(request(), { status: 201, statusText: 'Created' });

    const t = text(f);
    expect(t).toContain('na čekanju');
    expect(el(f).querySelector('form')).toBeNull();
  });

  it('podržava decimalni zarez', async () => {
    const f = await open(null);
    fill(f, '1,5');
    await submit(f);
    const req = http.expectOne(SUBMIT);
    expect(req.request.body).toEqual({ requestedBytes: 1.5 * GB });
    req.flush(request(), { status: 201, statusText: 'Created' });
  });

  it('MB se pretvara u bajtove', async () => {
    const f = await open(null);
    fill(f, '2048', MB);
    await submit(f);
    const req = http.expectOne(SUBMIT);
    expect(req.request.body).toEqual({ requestedBytes: 2048 * MB });
    req.flush(request(), { status: 201, statusText: 'Created' });
  });

  it('vrednost jednaka ili manja od trenutne kvote se ne šalje', async () => {
    const f = await open(null);
    for (const v of ['1', '0.5']) {
      fill(f, v);
      await submit(f);
      http.expectNone(SUBMIT);
      expect(text(f)).toContain('mora biti veća od trenutne');
    }
  });

  it('prazan, nenumerički i nepozitivan unos se ne šalje', async () => {
    const f = await open(null);
    for (const v of ['', '   ', 'abc', '-5', '0', '1e3', '5 GB']) {
      fill(f, v);
      await submit(f);
      http.expectNone(SUBMIT);
      expect(text(f)).toContain('pozitivan broj');
    }
  });

  it('prevelika vrednost se ne šalje', async () => {
    const f = await open(null);
    fill(f, '2000000000'); // ~2 * 10^9 GB > 1 PiB
    await submit(f);
    http.expectNone(SUBMIT);
    expect(text(f)).toContain('prevelika');
  });

  it('greška se ne prikazuje dok korisnik nije pokušao da pošalje', async () => {
    const f = await open(null);
    fill(f, 'abc');
    expect(el(f).querySelector('.zk__hint')).toBeNull();
  });

  it('409 prikazuje poruku servera i učitava zahtev na čekanju', async () => {
    const f = await open(null);
    fill(f, '5');
    await submit(f);
    http.expectOne(SUBMIT).flush({ error: 'već imaš zahtev na čekanju' }, { status: 409, statusText: 'Conflict' });

    http.expectOne(LATEST).flush(request()); // osvezen status
    await f.whenStable();
    f.detectChanges();
    expect(el(f).querySelector('.zk__status')).not.toBeNull();
    expect(el(f).querySelector('form')).toBeNull();
  });

  it('400 sa servera se prikazuje u formi, a unos ostaje', async () => {
    const f = await open(null);
    fill(f, '5');
    await submit(f);
    http.expectOne(SUBMIT).flush({ error: 'tražena kvota mora biti veća od trenutne' }, { status: 400, statusText: 'Bad Request' });
    f.detectChanges();

    expect(el(f).querySelector('[role=alert]')?.textContent).toContain('veća od trenutne');
    expect(el(f).querySelector<HTMLInputElement>('input')!.value).toBe('5');
  });

  it('dok traje slanje dugme je onemogućeno i zahtev se ne ponavlja', async () => {
    const f = await open(null);
    fill(f, '5');
    await submit(f);
    await submit(f);
    const req = http.expectOne(SUBMIT);
    expect(el(f).querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(true);
    req.flush(request(), { status: 201, statusText: 'Created' });
  });

  it('greška pri učitavanju statusa nudi ponovni pokušaj', async () => {
    const f = setup();
    http.expectOne(LATEST).flush({ error: 'interna greška' }, { status: 500, statusText: 'Server Error' });
    await f.whenStable();
    f.detectChanges();

    expect(el(f).querySelector('[role=alert]')).not.toBeNull();
    expect(el(f).querySelector('form')).toBeNull();
    el(f).querySelector<HTMLButtonElement>('button')!.click();
    http.expectOne(LATEST).flush(request());
  });
});
