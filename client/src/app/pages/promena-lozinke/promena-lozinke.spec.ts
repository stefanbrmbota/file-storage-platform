import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { PromenaLozinke } from './promena-lozinke';
import { ConfigService } from '../../core/config.service';
import { TokenService } from '../../core/auth/token.service';
import { fakeJwt } from '../../../testing/jwt';

const GW = 'http://gw.test';
const CURRENT = 'Stara-lozinka-1!';
const NEXT = 'Nova-lozinka-2@';

describe('Promena lozinke', () => {
  let http: HttpTestingController;
  let tokens: TokenService;

  function setup() {
    sessionStorage.clear();
    TestBed.configureTestingModule({
      imports: [PromenaLozinke],
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    tokens = TestBed.inject(TokenService);
    tokens.set(fakeJwt({ sub: 'u1', role: 'USER', exp: Math.floor(Date.now() / 1000) + 900 }));
    const fixture = TestBed.createComponent(PromenaLozinke);
    fixture.detectChanges();
    return fixture;
  }

  afterEach(() => http.verify());

  function type(fixture: ReturnType<typeof setup>, control: string, value: string) {
    const input = (fixture.nativeElement as HTMLElement).querySelector<HTMLInputElement>(`input[formControlName=${control}]`)!;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    input.dispatchEvent(new Event('blur'));
  }

  function fill(fixture: ReturnType<typeof setup>, overrides: Record<string, string> = {}) {
    const values: Record<string, string> = {
      currentPassword: CURRENT,
      newPassword: NEXT,
      newPasswordConfirm: NEXT,
      ...overrides,
    };
    for (const [control, value] of Object.entries(values)) {
      type(fixture, control, value);
    }
  }

  async function submit(fixture: ReturnType<typeof setup>) {
    (fixture.nativeElement as HTMLElement).querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
    await fixture.whenStable();
  }

  function text(fixture: ReturnType<typeof setup>): string {
    fixture.detectChanges();
    return (fixture.nativeElement as HTMLElement).textContent ?? '';
  }

  it('ispravna forma salje zahtev, pa brise token i trazi novu prijavu', async () => {
    const fixture = setup();
    expect(tokens.get()).not.toBeNull();
    fill(fixture);
    await submit(fixture);

    const req = http.expectOne(`${GW}/api/users/me/password`);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ currentPassword: CURRENT, newPassword: NEXT, newPasswordConfirm: NEXT });
    expect(req.request.urlWithParams).toBe(`${GW}/api/users/me/password`); // lozinke nikad u URL-u
    req.flush(null, { status: 204, statusText: 'No Content' });

    const el = fixture.nativeElement as HTMLElement;
    const t = text(fixture);
    expect(t).toContain('Lozinka je promenjena');
    expect(t).toContain('Prijavi se novom lozinkom');
    expect(el.querySelector('a[href="/prijava"]')).not.toBeNull();
    expect(el.querySelector('form')).toBeNull(); // lozinke ne ostaju na ekranu
    expect(tokens.get()).toBeNull(); // server je opozvao token, klijent ga zaboravlja
  });

  it('prazna forma se ne salje i prikazuje poruke', async () => {
    const fixture = setup();
    await submit(fixture);
    http.expectNone(`${GW}/api/users/me/password`);

    const t = text(fixture);
    expect(t).toContain('Unesi trenutnu lozinku.');
    expect(t).toContain('Unesi novu lozinku.');
    expect(t).toContain('Ponovi novu lozinku.');
    expect(tokens.get()).not.toBeNull();
  });

  it('slaba nova lozinka se ne salje i navodi sta fali', async () => {
    const fixture = setup();
    fill(fixture, { newPassword: 'abc', newPasswordConfirm: 'abc' });
    await submit(fixture);
    http.expectNone(`${GW}/api/users/me/password`);

    const t = text(fixture);
    expect(t).toContain('najmanje 8 karaktera');
    expect(t).toContain('veliko slovo');
    expect(t).toContain('broj');
    expect(t).toContain('specijalni znak');
  });

  it('razlicita ponovljena lozinka se ne salje', async () => {
    const fixture = setup();
    fill(fixture, { newPasswordConfirm: 'Druga-lozinka-3#' });
    await submit(fixture);
    http.expectNone(`${GW}/api/users/me/password`);
    expect(text(fixture)).toContain('Lozinke se ne poklapaju.');
  });

  it('nova lozinka ista kao trenutna se ne salje', async () => {
    const fixture = setup();
    fill(fixture, { newPassword: CURRENT, newPasswordConfirm: CURRENT });
    await submit(fixture);
    http.expectNone(`${GW}/api/users/me/password`);
    expect(text(fixture)).toContain('Nova lozinka mora biti drugačija od trenutne.');
  });

  it('pogresna trenutna lozinka (403) prikazuje poruku servera i ostavlja korisnika prijavljenog', async () => {
    const fixture = setup();
    fill(fixture);
    await submit(fixture);
    http.expectOne(`${GW}/api/users/me/password`).flush({ error: 'pogrešna trenutna lozinka' }, { status: 403, statusText: 'Forbidden' });

    const el = fixture.nativeElement as HTMLElement;
    expect(text(fixture)).toContain('pogrešna trenutna lozinka');
    expect(el.querySelector('[role=alert]')).not.toBeNull();
    expect(el.querySelector('form')).not.toBeNull();
    expect(tokens.get()).not.toBeNull(); // nista nije promenjeno, token ostaje
    expect(el.querySelector<HTMLInputElement>('input[formControlName=currentPassword]')!.value).toBe(CURRENT); // unos ostaje
  });

  it('prerano menjanje (409) prikazuje poruku servera sa vremenom', async () => {
    const fixture = setup();
    fill(fixture);
    await submit(fixture);
    http
      .expectOne(`${GW}/api/users/me/password`)
      .flush(
        { error: 'lozinka je nedavno promenjena; sledeća promena je moguća najranije 27.11.2026. u 09:30 (UTC)' },
        { status: 409, statusText: 'Conflict' },
      );
    expect(text(fixture)).toContain('27.11.2026. u 09:30 (UTC)');
    expect(tokens.get()).not.toBeNull();
  });

  it('greska pravila sa servera (400, npr. cesta lozinka) se prikazuje u formi', async () => {
    const fixture = setup();
    fill(fixture);
    await submit(fixture);
    http
      .expectOne(`${GW}/api/users/me/password`)
      .flush({ error: 'neispravan zahtev: lozinka je previše česta, izaberite drugu' }, { status: 400, statusText: 'Bad Request' });
    expect(text(fixture)).toContain('lozinka je previše česta, izaberite drugu');
    expect(tokens.get()).not.toBeNull();
  });

  it('dok traje slanje dugme je onemoguceno i zahtev se ne ponavlja', async () => {
    const fixture = setup();
    fill(fixture);
    await submit(fixture);
    await submit(fixture);
    const req = http.expectOne(`${GW}/api/users/me/password`);
    expect((fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(true);
    req.flush(null, { status: 204, statusText: 'No Content' });
  });

  it('ima link nazad na profil', () => {
    const fixture = setup();
    expect((fixture.nativeElement as HTMLElement).querySelector('a[href="/profil"]')).not.toBeNull();
  });
});
