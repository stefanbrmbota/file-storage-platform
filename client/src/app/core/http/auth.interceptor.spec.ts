import { TestBed } from '@angular/core/testing';
import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { authInterceptor, LOGIN_PATH } from './auth.interceptor';
import { ConfigService } from '../config.service';
import { TokenService } from '../auth/token.service';
import { ErrorBannerService } from '../notify/error-banner.service';
import { fakeJwt } from '../../../testing/jwt';

const GW = 'http://gw.test';

describe('authInterceptor', () => {
  let http: HttpClient;
  let ctrl: HttpTestingController;
  let tokens: TokenService;
  let router: Router;
  let banner: ErrorBannerService;
  const token = fakeJwt({ sub: 'u1', role: 'USER', exp: Math.floor(Date.now() / 1000) + 900 });

  beforeEach(() => {
    sessionStorage.clear();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        provideHttpClient(withInterceptors([authInterceptor])),
        provideHttpClientTesting(),
      ],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpClient);
    ctrl = TestBed.inject(HttpTestingController);
    tokens = TestBed.inject(TokenService);
    router = TestBed.inject(Router);
    banner = TestBed.inject(ErrorBannerService);
    vi.spyOn(router, 'navigate').mockResolvedValue(true);
  });

  afterEach(() => ctrl.verify());

  it('dodaje Bearer token na zahteve ka gateway-u', () => {
    tokens.set(token);
    http.get(`${GW}/api/notifications`).subscribe();
    expect(ctrl.expectOne(`${GW}/api/notifications`).request.headers.get('Authorization')).toBe(`Bearer ${token}`);
  });

  it('bez prijave ne salje Authorization', () => {
    http.get(`${GW}/health`).subscribe();
    expect(ctrl.expectOne(`${GW}/health`).request.headers.has('Authorization')).toBe(false);
  });

  it('nikad ne salje token na druge adrese', () => {
    tokens.set(token);
    http.get('https://neki-drugi-sajt.example/api').subscribe();
    http.get('http://gw.test.evil.example/api').subscribe(); // slican naziv, druga adresa
    expect(ctrl.expectOne('https://neki-drugi-sajt.example/api').request.headers.has('Authorization')).toBe(false);
    expect(ctrl.expectOne('http://gw.test.evil.example/api').request.headers.has('Authorization')).toBe(false);
  });

  it('na 401 brise token i vodi na prijavu sa returnUrl', () => {
    tokens.set(token);
    vi.spyOn(router, 'url', 'get').mockReturnValue('/obavestenja');
    let failed = false;
    http.get(`${GW}/api/notifications`).subscribe({ error: () => (failed = true) });
    ctrl.expectOne(`${GW}/api/notifications`).flush({ error: 'unauthorized' }, { status: 401, statusText: 'Unauthorized' });

    expect(failed).toBe(true);
    expect(tokens.get()).toBeNull();
    expect(router.navigate).toHaveBeenCalledWith([LOGIN_PATH], { queryParams: { returnUrl: '/obavestenja' } });
  });

  it('401 na prijavi (pogresna lozinka) ne preusmerava', () => {
    http.post(`${GW}/api/auth/login`, {}).subscribe({ error: () => undefined });
    ctrl.expectOne(`${GW}/api/auth/login`).flush({ error: 'pogresni podaci' }, { status: 401, statusText: 'Unauthorized' });
    expect(router.navigate).not.toHaveBeenCalled();
  });

  it('greske servera i mreze prikazuje u traci, a 404 ostavlja stranici', () => {
    http.get(`${GW}/api/notifications`).subscribe({ error: () => undefined });
    ctrl.expectOne(`${GW}/api/notifications`).flush({ error: 'servis trenutno nije dostupan' }, { status: 503, statusText: 'x' });
    expect(banner.message()).toBe('servis trenutno nije dostupan');

    banner.dismiss();
    http.get(`${GW}/api/folders/x/contents`).subscribe({ error: () => undefined });
    ctrl.expectOne(`${GW}/api/folders/x/contents`).flush({ error: 'folder not found' }, { status: 404, statusText: 'x' });
    expect(banner.message()).toBeNull();

    http.get(`${GW}/health`).subscribe({ error: () => undefined });
    ctrl.expectOne(`${GW}/health`).error(new ProgressEvent('error'), { status: 0 });
    expect(banner.message()).toContain('nije dostupan');
  });
});
