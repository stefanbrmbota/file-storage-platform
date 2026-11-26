import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { Title } from '@angular/platform-browser';
import { routes } from './app.routes';
import { ConfigService } from './core/config.service';
import { TokenService } from './core/auth/token.service';
import { fakeJwt } from '../testing/jwt';

describe('Rutiranje i zasticene rute (T-32.2, T-32.4)', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    sessionStorage.clear();
    TestBed.configureTestingModule({
      providers: [provideRouter(routes), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: 'http://gw.test' });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    // Zaglavlje prijavljenog korisnika pita za broj neprocitanih obavestenja (T-08.2).
    http.match((r) => r.url.endsWith('/api/notifications/unread-count')).forEach((r) => r.flush({ count: 0 }));
    http.verify();
  });

  const login = () =>
    TestBed.inject(TokenService).set(fakeJwt({ sub: 'u1', role: 'USER', exp: Math.floor(Date.now() / 1000) + 900 }));

  /** Otvara url kao prijavljen korisnik: guard pita gateway za sesiju. */
  async function openLoggedIn(url: string, session: { status: number; body: object } = { status: 200, body: { userId: "u1", role: "USER" } }) {
    const pending = RouterTestingHarness.create(url);
    await Promise.resolve();
    await new Promise((r) => setTimeout(r));
    http.expectOne('http://gw.test/api/auth/session').flush(session.body, { status: session.status, statusText: 'x' });
    return pending;
  }

  function text(harness: RouterTestingHarness): string {
    return ((harness.fixture.nativeElement as HTMLElement).textContent ?? '').replace(/\s+/g, ' ');
  }

  it('bez prijave / vodi na prijavu sa returnUrl', async () => {
    const harness = await RouterTestingHarness.create('/');
    const router = TestBed.inject(Router);
    expect(router.url).toBe('/prijava?returnUrl=%2F');
    expect(text(harness)).toContain('Prijava');
    // Zaglavlje nudi prijavu, a ne navigaciju i odjavu.
    const root = harness.fixture.nativeElement as HTMLElement;
    expect(root.querySelector('.login-link')).not.toBeNull();
    expect(root.querySelector('.logout')).toBeNull();
  });

  it('prijavljen korisnik vidi pocetnu u rasporedu, sa navigacijom i odjavom', async () => {
    login();
    const harness = await openLoggedIn('/');
    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
    harness.detectChanges();

    const root = harness.fixture.nativeElement as HTMLElement;
    expect(root.querySelector('header .brand')?.textContent).toContain('File Storage');
    expect(root.querySelector('nav a')?.textContent).toContain('Početna');
    expect(root.querySelector('.logout')).not.toBeNull();
    expect(text(harness)).toContain('Dobrodošli');
    expect(text(harness)).toContain('uloga: korisnik');
    expect(TestBed.inject(Title).getTitle()).toBe('Početna | File Storage');
  });

  it('prijavljen korisnik na /prijava se vraca na pocetnu', async () => {
    login();
    await openLoggedIn('/prijava'); // guestGuard vodi na /, a authGuard proverava sesiju
    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
    expect(TestBed.inject(Router).url).toBe('/');
  });

  it('opozvan token (odjava, pa vracen stari token) ne otvara stranicu', async () => {
    login();
    const harness = await openLoggedIn('/', { status: 401, body: { error: 'unauthorized' } });
    expect(TestBed.inject(Router).url).toBe('/prijava?returnUrl=%2F');
    expect(TestBed.inject(TokenService).get()).toBeNull();
    expect(text(harness)).not.toContain('Dobrodošli');
  });

  it('sesija se proverava jednom po tokenu', async () => {
    login();
    const harness = await openLoggedIn('/');
    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
    await harness.navigateByUrl('/nema');
    await harness.navigateByUrl('/');
    http.expectNone('http://gw.test/api/auth/session'); // vec potvrdjen
    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
  });

  it('zaglavlje prikazuje broj neprocitanih obavestenja (T-08.2)', async () => {
    login();
    const harness = await openLoggedIn('/');
    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
    http.match((r) => r.url.endsWith('/api/notifications/unread-count')).forEach((r) => r.flush({ count: 3 }));
    harness.detectChanges();

    const link = Array.from((harness.fixture.nativeElement as HTMLElement).querySelectorAll('nav a')).find((a) =>
      a.textContent?.includes('Obaveštenja'),
    );
    expect(link?.querySelector('.badge')?.textContent?.trim()).toBe('3');
  });

  it('nepostojeca adresa prikazuje 404, a zaglavlje ostaje', async () => {
    const harness = await RouterTestingHarness.create('/nepostojeca/stranica');
    const root = harness.fixture.nativeElement as HTMLElement;

    expect(root.querySelector('h1')?.textContent).toBe('404');
    expect(root.querySelector('header .brand')).not.toBeNull();
    expect(TestBed.inject(Title).getTitle()).toBe('Stranica ne postoji | File Storage');
  });

  it('link "Nazad na pocetnu" vodi na / bez ponovnog ucitavanja', async () => {
    login();
    const harness = await RouterTestingHarness.create('/nema');
    const link = (harness.fixture.nativeElement as HTMLElement).querySelector<HTMLAnchorElement>('main a[href="/"]');
    expect(link).not.toBeNull();

    link!.click();
    await new Promise((r) => setTimeout(r));
    http.expectOne('http://gw.test/api/auth/session').flush({ userId: 'u1', role: 'USER' });
    await harness.fixture.whenStable();
    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
    harness.detectChanges();

    expect(TestBed.inject(Router).url).toBe('/');
    expect(text(harness)).toContain('Dobrodošli');
  });
});
