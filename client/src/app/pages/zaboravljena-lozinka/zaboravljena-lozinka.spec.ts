import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { ZaboravljenaLozinka } from './zaboravljena-lozinka';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test';
const URL = `${GW}/api/auth/password-reset/request`;
const SERVER_MESSAGE = 'Ako nalog sa tom adresom postoji, poslali smo link za reset lozinke.';

describe('Zaboravljena lozinka', () => {
  let http: HttpTestingController;

  function setup() {
    TestBed.configureTestingModule({
      imports: [ZaboravljenaLozinka],
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    const fixture = TestBed.createComponent(ZaboravljenaLozinka);
    fixture.detectChanges();
    return fixture;
  }

  afterEach(() => http.verify());

  function typeEmail(fixture: ReturnType<typeof setup>, value: string) {
    const input = (fixture.nativeElement as HTMLElement).querySelector<HTMLInputElement>('input[type=email]')!;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    input.dispatchEvent(new Event('blur'));
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

  it('salje email (bez razmaka) i prikazuje poruku servera "Proverite mejl"', async () => {
    const fixture = setup();
    typeEmail(fixture, '  ana@example.com ');
    await submit(fixture);

    const req = http.expectOne(URL);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ email: 'ana@example.com' });
    expect(req.request.urlWithParams).toBe(URL); // email nikad u URL-u
    req.flush({ message: SERVER_MESSAGE }, { status: 202, statusText: 'Accepted' });

    const el = fixture.nativeElement as HTMLElement;
    const t = text(fixture);
    expect(t).toContain('Proverite mejl');
    expect(t).toContain(SERVER_MESSAGE);
    expect(el.querySelector('form')).toBeNull();
    expect(el.querySelector('a[href="/prijava"]')).not.toBeNull();
  });

  it('ako server ne posalje poruku, prikazuje opštu poruku', async () => {
    const fixture = setup();
    typeEmail(fixture, 'ana@example.com');
    await submit(fixture);
    http.expectOne(URL).flush({}, { status: 202, statusText: 'Accepted' });
    expect(text(fixture)).toContain('Ako nalog sa tom adresom postoji');
  });

  it('prikazana poruka ne otkriva da li nalog postoji (ista za svaki email)', async () => {
    const results: string[] = [];
    for (const email of ['postoji@example.com', 'nema@example.com']) {
      TestBed.resetTestingModule();
      const fixture = setup();
      typeEmail(fixture, email);
      await submit(fixture);
      http.expectOne(URL).flush({ message: SERVER_MESSAGE }, { status: 202, statusText: 'Accepted' });
      results.push((fixture.nativeElement as HTMLElement).textContent!.replace(/\s+/g, ' ').trim());
      http.verify();
    }
    expect(results[0]).toBe(results[1]);
  });

  it('prazan email se ne salje', async () => {
    const fixture = setup();
    await submit(fixture);
    http.expectNone(URL);
    expect(text(fixture)).toContain('Unesi email.');
  });

  it('neispravan email se ne salje', async () => {
    const fixture = setup();
    for (const bad of ['nije-email', 'a@b', 'ime <a@b.co>']) {
      typeEmail(fixture, bad);
      await submit(fixture);
      http.expectNone(URL);
      expect(text(fixture)).toContain('Unesi ispravan email');
    }
  });

  it('greska servera (400) se prikazuje u formi, a unos ostaje', async () => {
    const fixture = setup();
    typeEmail(fixture, 'ana@example.com');
    await submit(fixture);
    http.expectOne(URL).flush({ error: 'email nije ispravan' }, { status: 400, statusText: 'Bad Request' });

    const el = fixture.nativeElement as HTMLElement;
    expect(text(fixture)).toContain('email nije ispravan');
    expect(el.querySelector('[role=alert]')).not.toBeNull();
    expect(el.querySelector<HTMLInputElement>('input[type=email]')!.value).toBe('ana@example.com');
  });

  it('server nedostupan prikazuje opštu poruku', async () => {
    const fixture = setup();
    typeEmail(fixture, 'ana@example.com');
    await submit(fixture);
    http.expectOne(URL).error(new ProgressEvent('error'), { status: 0 });
    expect(text(fixture)).toContain('Server trenutno nije dostupan');
  });

  it('dok traje slanje dugme je onemoguceno i zahtev se ne ponavlja', async () => {
    const fixture = setup();
    typeEmail(fixture, 'ana@example.com');
    await submit(fixture);
    await submit(fixture);
    const req = http.expectOne(URL);
    expect((fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(true);
    req.flush({ message: SERVER_MESSAGE }, { status: 202, statusText: 'Accepted' });
  });

  it('ima link nazad na prijavu', () => {
    const fixture = setup();
    expect((fixture.nativeElement as HTMLElement).querySelector('a[href="/prijava"]')).not.toBeNull();
  });
});
