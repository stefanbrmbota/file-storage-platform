import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Account, Profil } from './profil';
import { formatSize } from '../folderi/files-api';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test';
const GIB = 1024 * 1024 * 1024;

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
    ...overrides,
  };
}

describe('Profil (kvota i zauzece)', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [Profil],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

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
});
