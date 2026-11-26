import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { DatePipe } from '@angular/common';
import { Account, Profil, remainingText } from './profil';
import { formatSize } from '../folderi/files-api';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test';
const GIB = 1024 * 1024 * 1024;

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
/** Zamrznuto "sada" (Date.now) u testovima, da rok lozinke ne zavisi od stvarnog datuma. */
const NOW = Date.UTC(2026, 10, 26, 12, 0, 0);

function account(overrides: Partial<Account> = {}): Account {
  return {
    id: 'u1',
    firstName: 'Ena',
    lastName: 'Testić',
    email: 'ena@example.com',
    role: 'USER',
    quotaBytes: GIB,
    usedBytes: 405264,
    remainingBytes: GIB - 405264,
    passwordExpiresAt: new Date(NOW + 60 * DAY).toISOString(),
    ...overrides,
  };
}

describe('Profil (kvota i zauzece)', () => {
  let http: HttpTestingController;
  let now = NOW;

  function setupTestBed() {
    TestBed.configureTestingModule({
      imports: [Profil],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
  }

  beforeEach(() => {
    now = NOW;
    vi.spyOn(Date, 'now').mockImplementation(() => now);
    setupTestBed();
  });

  afterEach(() => {
    // Ugradjeni zahtev za kvotu (US-21) pri otvaranju trazi status; ovde nema podnetog zahteva.
    http.match(`${GW}/api/users/me/quota-requests/latest`).forEach((r) => r.flush({ error: 'nema' }, { status: 404, statusText: 'Not Found' }));
    http.verify();
    vi.restoreAllMocks();
  });

  function render(a: Account) {
    const fixture = TestBed.createComponent(Profil);
    fixture.detectChanges();
    http.expectOne(`${GW}/api/users/me`).flush(a);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  const bar = (el: HTMLElement) => el.querySelector<HTMLElement>('.profil__bar')!;
  const fill = (el: HTMLElement) => el.querySelector<HTMLElement>('.profil__fill')!;

  it('zove GET /api/users/me bez ikakvog ID-ja i prikazuje nalog i kvotu', () => {
    const fixture = TestBed.createComponent(Profil);
    fixture.detectChanges();
    const req = http.expectOne(`${GW}/api/users/me`);
    expect(req.request.method).toBe('GET');
    expect(req.request.urlWithParams).toBe(`${GW}/api/users/me`);
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Učitavam');
    req.flush(account());
    fixture.detectChanges();

    const t = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(t).toContain('Ena Testić');
    expect(t).toContain('ena@example.com');
    expect(t).toContain('Korisnik');
    expect(t).toContain(formatSize(GIB)); // kvota
    expect(t).toContain(formatSize(405264)); // zauzeto
    expect(t).toContain(formatSize(GIB - 405264)); // preostalo
    expect(t).not.toContain('Učitavam');
  });

  it('prikazuje tri broja: kvota, zauzeto, preostalo', () => {
    const el = render(account({ quotaBytes: 1000, usedBytes: 250, remainingBytes: 750 }));
    const cards = Array.from(el.querySelectorAll('.profil__quota > div')).map((d) => [
      d.querySelector('dt')?.textContent?.trim(),
      d.querySelector('dd')?.textContent?.trim(),
    ]);
    expect(cards).toEqual([
      ['Kvota', '1000 B'],
      ['Zauzeto', '250 B'],
      ['Preostalo', '750 B'],
    ]);
  });

  it('traka pokazuje procenat zauzeca (50%)', () => {
    const el = render(account({ quotaBytes: 1000, usedBytes: 500, remainingBytes: 500 }));
    expect(el.textContent).toContain('(50.0%)');
    expect(fill(el).style.width).toBe('50%');
    expect(bar(el).getAttribute('role')).toBe('progressbar');
    expect(bar(el).getAttribute('aria-valuenow')).toBe('50.0');
    expect(fill(el).classList.contains('profil__fill--high')).toBe(false);
  });

  it('sasvim malo zauzece se prikazuje kao "<0.1%", a ne kao 0', () => {
    const el = render(account()); // 405264 od 1 GiB je oko 0.04%
    expect(el.textContent).toContain('(<0.1%)');
  });

  it('prazno skladiste je 0%', () => {
    const el = render(account({ usedBytes: 0, remainingBytes: GIB }));
    expect(el.textContent).toContain('(0%)');
    expect(fill(el).style.width).toBe('0%');
  });

  it('od 90% traka je upozoravajuca (crvena)', () => {
    const el = render(account({ quotaBytes: 1000, usedBytes: 900, remainingBytes: 100 }));
    expect(fill(el).classList.contains('profil__fill--high')).toBe(true);
    const below = TestBed.createComponent(Profil);
    below.detectChanges();
    http.expectOne(`${GW}/api/users/me`).flush(account({ quotaBytes: 1000, usedBytes: 899, remainingBytes: 101 }));
    below.detectChanges();
    expect((below.nativeElement as HTMLElement).querySelector('.profil__fill--high')).toBeNull();
  });

  it('zauzece vece od kvote: traka je puna (100%), preostalo 0 B', () => {
    const el = render(account({ quotaBytes: 1000, usedBytes: 1500, remainingBytes: 0 }));
    expect(el.textContent).toContain('(100.0%)');
    expect(fill(el).style.width).toBe('100%');
    expect(el.textContent).toContain('0 B');
  });

  it('kvota 0 ne daje NaN niti deljenje nulom', () => {
    const el = render(account({ quotaBytes: 0, usedBytes: 0, remainingBytes: 0 }));
    expect(el.textContent).not.toContain('NaN');
    expect(el.textContent).not.toContain('Infinity');
    expect(el.textContent).toContain('(0%)');
  });

  it('ima link za promenu lozinke (US-14)', () => {
    const el = render(account());
    expect(el.querySelector('a[href="/promena-lozinke"]')?.textContent).toContain('Promeni lozinku');
  });

  it('administrator ima oznaku uloge', () => {
    const el = render(account({ role: 'ADMIN' }));
    expect(el.textContent).toContain('Administrator');
  });

  it('podaci sa servera se prikazuju kao tekst (bez HTML-a)', () => {
    const el = render(account({ firstName: '<img src=x onerror=alert(1)>', lastName: '<b>Zlo</b>' }));
    expect(el.querySelector('img')).toBeNull();
    expect(el.querySelector('.profil__data b')).toBeNull();
    expect(el.textContent).toContain('<img src=x onerror=alert(1)>');
  });

  it('greska servera se prikazuje, a "Pokusaj ponovo" ponovo trazi podatke', () => {
    const fixture = TestBed.createComponent(Profil);
    fixture.detectChanges();
    http.expectOne(`${GW}/api/users/me`).flush({ error: 'nalog nije pronađen' }, { status: 404, statusText: 'Not Found' });
    fixture.detectChanges();

    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('[role=alert]')?.textContent).toContain('nalog nije pronađen');
    expect(el.querySelector('.profil__bar')).toBeNull();

    el.querySelector<HTMLButtonElement>('button')!.click();
    http.expectOne(`${GW}/api/users/me`).flush(account());
    fixture.detectChanges();
    expect(el.querySelector('[role=alert]')).toBeNull();
    expect(el.textContent).toContain('Ena Testić');
  });

  it('server nedostupan prikazuje opštu poruku', () => {
    const fixture = TestBed.createComponent(Profil);
    fixture.detectChanges();
    http.expectOne(`${GW}/api/users/me`).error(new ProgressEvent('error'), { status: 0 });
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('[role=alert]')?.textContent).toContain('Server trenutno nije dostupan');
  });

  // ---- Rok lozinke (US-16, T-16.4) ----

  /** Rok lozinke za nalog: istice za `ms` milisekundi od zamrznutog "sada". */
  const expiresIn = (ms: number) => new Date(NOW + ms).toISOString();

  const passwordBox = (el: HTMLElement) => el.querySelector<HTMLElement>('.profil__password');

  const humanDate = (iso: string) => new DatePipe('en-US').transform(iso, 'dd.MM.yyyy. HH:mm');

  it('prikazuje do kada lozinka vazi i koliko je jos ostalo', () => {
    const iso = expiresIn(40 * DAY + 5 * HOUR);
    const el = render(account({ passwordExpiresAt: iso }));
    const box = passwordBox(el)!;
    expect(box.textContent).toContain(`Lozinka važi do ${humanDate(iso)}`);
    expect(box.textContent).toContain('Još 40 dana.');
    expect(box.classList.contains('profil__password--warn')).toBe(false);
    expect(box.classList.contains('profil__password--expired')).toBe(false);
    expect(box.getAttribute('role')).toBeNull(); // bez upozorenja nema alarma
  });

  it('upozorava u poslednjih 7 dana (granica ukljucena)', () => {
    const el = render(account({ passwordExpiresAt: expiresIn(7 * DAY) }));
    const box = passwordBox(el)!;
    expect(box.classList.contains('profil__password--warn')).toBe(true);
    expect(box.getAttribute('role')).toBe('alert');
    expect(box.textContent).toContain('Još 7 dana.');
  });

  it('minut preko granice od 7 dana jos nije upozorenje', () => {
    const el = render(account({ passwordExpiresAt: expiresIn(7 * DAY + MINUTE) }));
    expect(passwordBox(el)!.classList.contains('profil__password--warn')).toBe(false);
  });

  it('skraceni period za odbranu: preostalo u satima i minutima', () => {
    expect(passwordBox(render(account({ passwordExpiresAt: expiresIn(3 * HOUR + 10 * MINUTE) })))!.textContent).toContain('Još 3 sata.');
    TestBed.resetTestingModule();
    setupTestBed();
    expect(passwordBox(render(account({ passwordExpiresAt: expiresIn(90_000) })))!.textContent).toContain('Još 1 minut.');
    TestBed.resetTestingModule();
    setupTestBed();
    const el = render(account({ passwordExpiresAt: expiresIn(20_000) }));
    expect(passwordBox(el)!.textContent).toContain('Još manje od minut.');
    expect(passwordBox(el)!.classList.contains('profil__password--warn')).toBe(true);
  });

  it('istekla lozinka: poruka da se promeni odmah, uz crveno upozorenje', () => {
    const el = render(account({ passwordExpiresAt: expiresIn(-HOUR) }));
    const box = passwordBox(el)!;
    expect(box.classList.contains('profil__password--expired')).toBe(true);
    expect(box.getAttribute('role')).toBe('alert');
    expect(box.textContent).toContain('Lozinka je istekla');
    expect(box.textContent).toContain('Zaboravio si lozinku?');
    expect(el.querySelector('a[href="/promena-lozinke"]')).not.toBeNull(); // promena je ponudjena
  });

  it('tacno u trenutku isteka lozinka se tretira kao istekla', () => {
    const el = render(account({ passwordExpiresAt: expiresIn(0) }));
    expect(passwordBox(el)!.classList.contains('profil__password--expired')).toBe(true);
  });

  it('bez roka u odgovoru (stariji server) nema prikaza roka, a profil radi', () => {
    const el = render(account({ passwordExpiresAt: undefined }));
    expect(passwordBox(el)).toBeNull();
    expect(el.textContent).not.toContain('Lozinka važi do');
    expect(el.textContent).toContain('Ena Testić');
    expect(el.querySelector('a[href="/promena-lozinke"]')).not.toBeNull();
  });

  it('neispravan rok u odgovoru se ignorise (nema NaN ni "Invalid Date")', () => {
    const el = render(account({ passwordExpiresAt: 'nije-datum' }));
    expect(passwordBox(el)).toBeNull();
    expect(el.textContent).not.toContain('NaN');
    expect(el.textContent).not.toContain('Invalid');
  });

  it('ponovno ucitavanje racuna preostalo vreme od novog "sada"', () => {
    const fixture = TestBed.createComponent(Profil);
    fixture.detectChanges();
    http.expectOne(`${GW}/api/users/me`).flush({ error: 'x' }, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();

    now = NOW + 2 * DAY; // dok korisnik cita gresku, vreme prolazi
    (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('button')!.click();
    http.expectOne(`${GW}/api/users/me`).flush(account({ passwordExpiresAt: expiresIn(10 * DAY) }));
    fixture.detectChanges();
    expect(passwordBox(fixture.nativeElement as HTMLElement)!.textContent).toContain('Još 8 dana.');
  });
});

describe('remainingText (srpski oblici)', () => {
  it('dani', () => {
    const t = (n: number) => remainingText(n * DAY);
    expect([t(1), t(2), t(4), t(5), t(11), t(12), t(21), t(22), t(25), t(60)]).toEqual([
      '1 dan', '2 dana', '4 dana', '5 dana', '11 dana', '12 dana', '21 dan', '22 dana', '25 dana', '60 dana',
    ]);
  });

  it('sati', () => {
    const t = (n: number) => remainingText(n * HOUR);
    expect([t(1), t(2), t(4), t(5), t(11), t(21), t(23)]).toEqual(['1 sat', '2 sata', '4 sata', '5 sati', '11 sati', '21 sat', '23 sata']);
  });

  it('minuti', () => {
    const t = (n: number) => remainingText(n * MINUTE);
    expect([t(1), t(2), t(5), t(21), t(59)]).toEqual(['1 minut', '2 minuta', '5 minuta', '21 minut', '59 minuta']);
  });

  it('zaokruzuje nadole na najkrupniju jedinicu', () => {
    expect(remainingText(DAY + 23 * HOUR)).toBe('1 dan');
    expect(remainingText(HOUR + 59 * MINUTE)).toBe('1 sat');
    expect(remainingText(MINUTE + 59_000)).toBe('1 minut');
  });

  it('manje od minuta', () => {
    expect(remainingText(59_999)).toBe('manje od minut');
    expect(remainingText(1)).toBe('manje od minut');
  });
});
