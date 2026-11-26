import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { ResetLozinke } from './reset-lozinke';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test';
const URL = `${GW}/api/auth/password-reset/confirm`;
const TOKEN = 'aB3_-xYz0123456789ABCDEFGHIJKLMNOPQRSTUV';
const NEXT = 'Nova-lozinka-2@';

describe('Reset lozinke', () => {
  let http: HttpTestingController;
  let router: Router;

  async function open(url: string) {
    TestBed.configureTestingModule({
      providers: [provideRouter([{ path: 'reset-lozinke', component: ResetLozinke }]), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    router = TestBed.inject(Router);
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl(url, ResetLozinke);
    await harness.fixture.whenStable();
    harness.detectChanges();
    return harness;
  }

  afterEach(() => http.verify());

  const root = (h: RouterTestingHarness) => h.routeNativeElement as HTMLElement;

  function text(h: RouterTestingHarness): string {
    h.detectChanges();
    return root(h).textContent ?? '';
  }

  function type(h: RouterTestingHarness, control: string, value: string) {
    const input = root(h).querySelector<HTMLInputElement>(`input[formControlName=${control}]`)!;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    input.dispatchEvent(new Event('blur'));
  }

  function fill(h: RouterTestingHarness, overrides: Record<string, string> = {}) {
    const values: Record<string, string> = { newPassword: NEXT, newPasswordConfirm: NEXT, ...overrides };
    for (const [control, value] of Object.entries(values)) {
      type(h, control, value);
    }
  }

  async function submit(h: RouterTestingHarness) {
    root(h).querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    h.detectChanges();
    await h.fixture.whenStable();
  }

  it('token iz fragmenta se uklanja iz adrese, pa se salje u telu zajedno sa lozinkom', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    expect(router.url).toBe('/reset-lozinke'); // fragment sa tokenom je uklonjen
    http.expectNone(URL); // nista se ne salje dok korisnik ne unese lozinku
    expect(root(h).querySelector('form')).not.toBeNull();

    fill(h);
    await submit(h);
    const req = http.expectOne(URL);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ token: TOKEN, newPassword: NEXT, newPasswordConfirm: NEXT });
    expect(req.request.urlWithParams).not.toContain(TOKEN); // token nikad u URL-u zahteva
    expect(req.request.urlWithParams).not.toContain(NEXT);
    req.flush(null, { status: 204, statusText: 'No Content' });

    const t = text(h);
    expect(t).toContain('Lozinka je promenjena');
    expect(t).toContain('Možeš da se prijaviš novom lozinkom');
    expect(root(h).querySelector('a[href="/prijava"]')).not.toBeNull();
    expect(root(h).querySelector('form')).toBeNull(); // lozinke ne ostaju na ekranu
    expect(t).not.toContain(TOKEN);
  });

  it('bez tokena u linku nema forme i nudi novi zahtev', async () => {
    const h = await open('/reset-lozinke');
    http.expectNone(URL);
    expect(text(h)).toContain('Link za reset nije ispravan');
    expect(root(h).querySelector('form')).toBeNull();
    expect(root(h).querySelector('a[href="/zaboravljena-lozinka"]')).not.toBeNull();
  });

  it('fragment bez parametra token se ignorise', async () => {
    const h = await open('/reset-lozinke#neki=drugi');
    expect(text(h)).toContain('Link za reset nije ispravan');
    expect(root(h).querySelector('form')).toBeNull();
  });

  it('prazna forma se ne salje i prikazuje poruke', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    await submit(h);
    http.expectNone(URL);
    const t = text(h);
    expect(t).toContain('Unesi novu lozinku.');
    expect(t).toContain('Ponovi novu lozinku.');
  });

  it('slaba lozinka se ne salje i navodi sta fali', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    fill(h, { newPassword: 'abc', newPasswordConfirm: 'abc' });
    await submit(h);
    http.expectNone(URL);
    const t = text(h);
    expect(t).toContain('najmanje 8 karaktera');
    expect(t).toContain('veliko slovo');
    expect(t).toContain('broj');
    expect(t).toContain('specijalni znak');
  });

  it('razlicita ponovljena lozinka se ne salje', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    fill(h, { newPasswordConfirm: 'Druga-lozinka-3#' });
    await submit(h);
    http.expectNone(URL);
    expect(text(h)).toContain('Lozinke se ne poklapaju.');
  });

  it('nevazeci ili iskoriscen link (400) prikazuje poruku servera, uz link za novi zahtev', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    fill(h);
    await submit(h);
    http
      .expectOne(URL)
      .flush({ error: 'link je nevažeći, istekao ili je već iskorišćen' }, { status: 400, statusText: 'Bad Request' });

    expect(text(h)).toContain('link je nevažeći, istekao ili je već iskorišćen');
    expect(root(h).querySelector('[role=alert]')).not.toBeNull();
    expect(root(h).querySelector('a[href="/zaboravljena-lozinka"]')).not.toBeNull();
  });

  it('greska pravila sa servera (cesta lozinka) se prikazuje, a ponovni pokusaj istim tokenom radi', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    fill(h);
    await submit(h);
    http
      .expectOne(URL)
      .flush({ error: 'neispravan zahtev: lozinka je previše česta, izaberite drugu' }, { status: 400, statusText: 'Bad Request' });
    expect(text(h)).toContain('lozinka je previše česta');
    expect(root(h).querySelector('form')).not.toBeNull();

    fill(h, { newPassword: 'Druga-lozinka-3#', newPasswordConfirm: 'Druga-lozinka-3#' });
    await submit(h);
    const again = http.expectOne(URL);
    expect(again.request.body).toEqual({ token: TOKEN, newPassword: 'Druga-lozinka-3#', newPasswordConfirm: 'Druga-lozinka-3#' });
    again.flush(null, { status: 204, statusText: 'No Content' });
    expect(text(h)).toContain('Lozinka je promenjena');
  });

  it('istovremena promena (409) se prikazuje, a unos ostaje', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    fill(h);
    await submit(h);
    http.expectOne(URL).flush({ error: 'lozinka je upravo promenjena, pokušajte ponovo' }, { status: 409, statusText: 'Conflict' });
    expect(text(h)).toContain('pokušajte ponovo');
    expect(root(h).querySelector<HTMLInputElement>('input[formControlName=newPassword]')!.value).toBe(NEXT);
  });

  it('novi link u istoj kartici zamenjuje token i cisti poruku o gresci', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    fill(h);
    await submit(h);
    http.expectOne(URL).flush({ error: 'link je nevažeći, istekao ili je već iskorišćen' }, { status: 400, statusText: 'Bad Request' });
    h.detectChanges();
    expect(root(h).querySelector('[role=alert]')).not.toBeNull();

    await router.navigateByUrl('/reset-lozinke#token=drugi-token');
    await h.fixture.whenStable();
    h.detectChanges();
    expect(root(h).querySelector('[role=alert]')).toBeNull();
    expect(router.url).toBe('/reset-lozinke');

    fill(h);
    await submit(h);
    const req = http.expectOne(URL);
    expect(req.request.body.token).toBe('drugi-token');
    req.flush(null, { status: 204, statusText: 'No Content' });
  });

  it('dok traje slanje dugme je onemoguceno i zahtev se ne ponavlja', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    fill(h);
    await submit(h);
    await submit(h);
    const req = http.expectOne(URL);
    expect(root(h).querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(true);
    req.flush(null, { status: 204, statusText: 'No Content' });
  });

  it('uvek nudi link za novi zahtev', async () => {
    const h = await open(`/reset-lozinke#token=${TOKEN}`);
    expect(root(h).querySelector('a[href="/zaboravljena-lozinka"]')).not.toBeNull();
  });
});
