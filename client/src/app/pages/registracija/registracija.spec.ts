import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Registracija } from './registracija';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test';
const OK_PASSWORD = 'Tajna-lozinka1';

describe('Registracija', () => {
  let http: HttpTestingController;

  function setup() {
    TestBed.configureTestingModule({
      imports: [Registracija],
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    const fixture = TestBed.createComponent(Registracija);
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
      firstName: '  Ana ',
      lastName: 'Anić',
      email: ' ana@example.com ',
      password: OK_PASSWORD,
      passwordConfirm: OK_PASSWORD,
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

  it('ispravna forma salje zahtev (bez razmaka na krajevima) i prikazuje "Proverite mejl"', async () => {
    const fixture = setup();
    fill(fixture);
    await submit(fixture);

    const req = http.expectOne(`${GW}/api/auth/register`);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({
      firstName: 'Ana',
      lastName: 'Anić',
      email: 'ana@example.com',
      password: OK_PASSWORD,
      passwordConfirm: OK_PASSWORD,
    });
    expect(req.request.url).not.toContain(OK_PASSWORD); // lozinka nikad u URL-u
    req.flush({ id: 'u1', email: 'ana@example.com' }, { status: 201, statusText: 'Created' });

    const t = text(fixture);
    expect(t).toContain('Proverite mejl');
    expect(t).toContain('ana@example.com');
    expect((fixture.nativeElement as HTMLElement).querySelector('form')).toBeNull(); // lozinka ne ostaje na ekranu
  });

  it('prazna forma se ne salje i prikazuje poruke', async () => {
    const fixture = setup();
    await submit(fixture);
    http.expectNone(`${GW}/api/auth/register`);

    const t = text(fixture);
    expect(t).toContain('Unesi ime.');
    expect(t).toContain('Unesi prezime.');
    expect(t).toContain('Unesi email.');
    expect(t).toContain('Unesi lozinku.');
  });

  it('slaba lozinka se ne salje i navodi sta fali', async () => {
    const fixture = setup();
    fill(fixture, { password: 'abc', passwordConfirm: 'abc' });
    await submit(fixture);
    http.expectNone(`${GW}/api/auth/register`);

    const t = text(fixture);
    expect(t).toContain('najmanje 8 karaktera');
    expect(t).toContain('veliko slovo');
    expect(t).toContain('broj');
    expect(t).toContain('specijalni znak');
    expect(t).not.toContain('malo slovo');
  });

  it('lozinka bez specijalnog znaka je odbijena', async () => {
    const fixture = setup();
    fill(fixture, { password: 'Tajnalozinka1', passwordConfirm: 'Tajnalozinka1' });
    await submit(fixture);
    http.expectNone(`${GW}/api/auth/register`);
    expect(text(fixture)).toContain('specijalni znak');
  });

  it('razlicita ponovljena lozinka se ne salje', async () => {
    const fixture = setup();
    fill(fixture, { passwordConfirm: 'Druga-lozinka2' });
    await submit(fixture);
    http.expectNone(`${GW}/api/auth/register`);
    expect(text(fixture)).toContain('Lozinke se ne poklapaju.');
  });

  it('neispravan email i predugacko ime se ne salju', async () => {
    const fixture = setup();
    fill(fixture, { email: 'nije-email', firstName: 'A'.repeat(51) });
    await submit(fixture);
    http.expectNone(`${GW}/api/auth/register`);

    const t = text(fixture);
    expect(t).toContain('Unesi ispravan email');
    expect(t).toContain('Ime može imati najviše 50 karaktera.');
  });

  it('zauzet email (409) prikazuje poruku servera u formi i cuva unos', async () => {
    const fixture = setup();
    fill(fixture);
    await submit(fixture);
    http.expectOne(`${GW}/api/auth/register`).flush({ error: 'email je već zauzet' }, { status: 409, statusText: 'Conflict' });

    const el = fixture.nativeElement as HTMLElement;
    expect(text(fixture)).toContain('email je već zauzet');
    expect(el.querySelector('[role=alert]')).not.toBeNull();
    expect(el.querySelector('form')).not.toBeNull();
    expect(el.querySelector<HTMLInputElement>('input[formControlName=firstName]')!.value).toBe('  Ana ');
  });

  it('greska validacije sa servera (400) prikazuje poruku servera', async () => {
    const fixture = setup();
    fill(fixture);
    await submit(fixture);
    http
      .expectOne(`${GW}/api/auth/register`)
      .flush({ error: 'neispravan zahtev: lozinka je previše česta, izaberite drugu' }, { status: 400, statusText: 'Bad Request' });

    expect(text(fixture)).toContain('lozinka je previše česta, izaberite drugu');
  });

  it('dok traje slanje dugme je onemoguceno i zahtev se ne ponavlja', async () => {
    const fixture = setup();
    fill(fixture);
    await submit(fixture);
    await submit(fixture); // drugi submit dok prvi traje
    const req = http.expectOne(`${GW}/api/auth/register`);
    expect((fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(true);
    req.flush({}, { status: 201, statusText: 'Created' });
  });
});
