import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { MagicniLink } from './magicni-link';
import { ConfigService } from '../../core/config.service';
import { TokenService } from '../../core/auth/token.service';
import { fakeJwt } from '../../../testing/jwt';

const GW = 'http://gw.test';
const URL = `${GW}/api/auth/magic-link/login`;
const TOKEN = 'aB3_-xYz0123456789ABCDEFGHIJKLMNOPQRSTUV';

describe('Prijava magičnim linkom', () => {
  let http: HttpTestingController;
  let router: Router;
  let navigate: ReturnType<typeof vi.spyOn>;

  async function open(url: string) {
    sessionStorage.clear();
    TestBed.configureTestingModule({
      providers: [provideRouter([{ path: 'magicni-link', component: MagicniLink }]), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    router = TestBed.inject(Router);
    navigate = vi.spyOn(router, 'navigateByUrl').mockResolvedValue(true);
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl(url, MagicniLink);
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

  const validJwt = () => fakeJwt({ sub: 'u1', role: 'USER', exp: Math.floor(Date.now() / 1000) + 900 });

  it('cita token iz fragmenta, salje ga u telu, cuva JWT i vodi na pocetnu', async () => {
    const h = await open(`/magicni-link#token=${TOKEN}`);

    const req = http.expectOne(URL);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ token: TOKEN });
    expect(req.request.urlWithParams).toBe(URL); // token nikad u URL-u
    const jwt = validJwt();
    req.flush({ token: jwt });
    await h.fixture.whenStable();

    expect(TestBed.inject(TokenService).get()).toBe(jwt);
    expect(navigate).toHaveBeenCalledWith('/');
  });

  it('uklanja token iz adrese', async () => {
    await open(`/magicni-link#token=${TOKEN}`);
    http.expectOne(URL).flush({ token: validJwt() });
    expect(router.url).not.toContain(TOKEN);
    expect(router.url).not.toContain('token=');
  });

  it('dok traje prijava prikazuje poruku o prijavljivanju', async () => {
    const h = await open(`/magicni-link#token=${TOKEN}`);
    expect(text(h)).toContain('Prijavljujem te');
    http.expectOne(URL).flush({ token: validJwt() });
  });

  it('nevazeci, istekao ili iskoriscen link (401) prikazuje gresku i vezu za novi zahtev', async () => {
    const h = await open(`/magicni-link#token=${TOKEN}`);
    http.expectOne(URL).flush({ error: 'link je nevažeći, istekao ili je već iskorišćen' }, { status: 401, statusText: 'Unauthorized' });

    const el = root(h);
    expect(text(h)).toContain('link je nevažeći, istekao ili je već iskorišćen');
    expect(el.querySelector('[role=alert]')).not.toBeNull();
    expect(el.querySelector('a[href="/povracaj-naloga"]')).not.toBeNull();
    expect(el.querySelector('button')).toBeNull(); // 401 se ne ponavlja
    expect(TestBed.inject(TokenService).get()).toBeNull();
    expect(navigate).not.toHaveBeenCalled();
  });

  it('bez tokena u adresi ne zove server i nudi novi link', async () => {
    const h = await open('/magicni-link');
    http.expectNone(URL);
    expect(text(h)).toContain('Link nije ispravan');
    expect(root(h).querySelector('a[href="/povracaj-naloga"]')).not.toBeNull();
  });

  it('prazan token u fragmentu se ne salje', async () => {
    const h = await open('/magicni-link#token=');
    http.expectNone(URL);
    expect(text(h)).toContain('Link nije ispravan');
  });

  it('odgovor bez ispravnog JWT-a se ne tretira kao prijava', async () => {
    const h = await open(`/magicni-link#token=${TOKEN}`);
    http.expectOne(URL).flush({ token: 'nije-jwt' });

    expect(text(h)).toContain('odgovor servera nije ispravan');
    expect(TestBed.inject(TokenService).get()).toBeNull();
    expect(navigate).not.toHaveBeenCalled();
  });

  it('server nedostupan (502) nudi ponovni pokusaj istim tokenom', async () => {
    const h = await open(`/magicni-link#token=${TOKEN}`);
    http.expectOne(URL).flush({ error: 'servis nije dostupan' }, { status: 502, statusText: 'Bad Gateway' });

    const retry = root(h).querySelector<HTMLButtonElement>('button')!;
    expect(retry).not.toBeNull();
    retry.click();

    const again = http.expectOne(URL);
    expect(again.request.body).toEqual({ token: TOKEN });
    again.flush({ token: validJwt() });
    await h.fixture.whenStable();
    expect(navigate).toHaveBeenCalledWith('/');
  });

  it('JWT se ne upisuje u adresu ni kad prijava uspe', async () => {
    await open(`/magicni-link#token=${TOKEN}`);
    const jwt = validJwt();
    http.expectOne(URL).flush({ token: jwt });
    expect(router.url).not.toContain(jwt);
  });
});
