import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { Potvrda } from './potvrda';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test';
const TOKEN = 'aB3_-xYz0123456789ABCDEFGHIJKLMNOPQRSTUV';

describe('Potvrda naloga', () => {
  let http: HttpTestingController;
  let router: Router;

  async function open(url: string) {
    TestBed.configureTestingModule({
      providers: [provideRouter([{ path: 'potvrda', component: Potvrda }]), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    router = TestBed.inject(Router);
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl(url, Potvrda);
    return harness;
  }

  afterEach(() => http.verify());

  function text(harness: RouterTestingHarness): string {
    harness.detectChanges();
    return (harness.routeNativeElement as HTMLElement).textContent ?? '';
  }

  it('cita token iz fragmenta, uklanja ga iz adrese i salje ga u telu zahteva', async () => {
    const harness = await open(`/potvrda#token=${TOKEN}`);
    await harness.fixture.whenStable();

    const req = http.expectOne(`${GW}/api/auth/confirm`);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ token: TOKEN });
    expect(req.request.url).not.toContain(TOKEN); // token nikad u URL-u zahteva
    expect(req.request.urlWithParams).not.toContain(TOKEN);
    expect(router.url).toBe('/potvrda'); // fragment sa tokenom je uklonjen iz adrese
    expect(text(harness)).toContain('Potvrđujem nalog');

    req.flush({ message: 'nalog je aktiviran' });
    const t = text(harness);
    expect(t).toContain('Nalog je aktiviran');
    expect((harness.routeNativeElement as HTMLElement).querySelector('a[href="/prijava"]')).not.toBeNull();
    expect(t).not.toContain(TOKEN);
  });

  it('nevazeci ili iskoriscen link (400) prikazuje poruku servera bez ponovnog pokusaja', async () => {
    const harness = await open(`/potvrda#token=${TOKEN}`);
    await harness.fixture.whenStable();
    http
      .expectOne(`${GW}/api/auth/confirm`)
      .flush({ error: 'link je nevažeći, istekao ili je već iskorišćen' }, { status: 400, statusText: 'Bad Request' });

    const t = text(harness);
    expect(t).toContain('link je nevažeći, istekao ili je već iskorišćen');
    expect(t).not.toContain('Pokušaj ponovo');
  });

  it('servis nedostupan (502) nudi ponovni pokusaj sa istim tokenom', async () => {
    const harness = await open(`/potvrda#token=${TOKEN}`);
    await harness.fixture.whenStable();
    http
      .expectOne(`${GW}/api/auth/confirm`)
      .flush({ error: 'servis je privremeno nedostupan, pokušajte ponovo' }, { status: 502, statusText: 'Bad Gateway' });

    expect(text(harness)).toContain('servis je privremeno nedostupan');
    const retry = (harness.routeNativeElement as HTMLElement).querySelector<HTMLButtonElement>('button')!;
    expect(retry.textContent).toContain('Pokušaj ponovo');
    retry.click();

    const again = http.expectOne(`${GW}/api/auth/confirm`);
    expect(again.request.body).toEqual({ token: TOKEN });
    again.flush({ message: 'nalog je aktiviran' });
    expect(text(harness)).toContain('Nalog je aktiviran');
  });

  it('bez tokena u linku ne salje zahtev i javlja da link nije ispravan', async () => {
    const harness = await open('/potvrda');
    await harness.fixture.whenStable();
    http.expectNone(`${GW}/api/auth/confirm`);
    expect(text(harness)).toContain('Link za potvrdu nije ispravan');
  });

  it('novi link u istoj kartici se potvrdjuje iznova (stara poruka se ne zadrzava)', async () => {
    const harness = await open(`/potvrda#token=${TOKEN}`);
    await harness.fixture.whenStable();
    http.expectOne(`${GW}/api/auth/confirm`).flush({ message: 'nalog je aktiviran' });
    expect(text(harness)).toContain('Nalog je aktiviran');
    expect(router.url).toBe('/potvrda');

    await router.navigateByUrl('/potvrda#token=drugi-token');
    await harness.fixture.whenStable();
    const req = http.expectOne(`${GW}/api/auth/confirm`);
    expect(req.request.body).toEqual({ token: 'drugi-token' });
    req.flush({ error: 'link je nevažeći, istekao ili je već iskorišćen' }, { status: 400, statusText: 'Bad Request' });

    expect(text(harness)).toContain('link je nevažeći, istekao ili je već iskorišćen');
    expect(router.url).toBe('/potvrda'); // i drugi token je uklonjen iz adrese
  });

  it('fragment bez parametra token se ignorise', async () => {
    const harness = await open('/potvrda#neki=drugi');
    await harness.fixture.whenStable();
    http.expectNone(`${GW}/api/auth/confirm`);
    expect(text(harness)).toContain('Link za potvrdu nije ispravan');
  });
});
