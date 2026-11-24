import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { AuthService } from './auth.service';
import { TokenService } from './token.service';
import { ConfigService } from '../config.service';
import { fakeJwt } from '../../../testing/jwt';

const GW = 'http://gw.test';

describe('AuthService.logout', () => {
  let http: HttpTestingController;
  let tokens: TokenService;
  let router: Router;

  beforeEach(() => {
    sessionStorage.clear();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    tokens = TestBed.inject(TokenService);
    router = TestBed.inject(Router);
    vi.spyOn(router, 'navigate').mockResolvedValue(true);
    tokens.set(fakeJwt({ sub: 'u1', role: 'USER', exp: Math.floor(Date.now() / 1000) + 900 }));
  });

  afterEach(() => http.verify());

  it('poziva /api/auth/logout, brise token i vodi na prijavu', () => {
    TestBed.inject(AuthService).logout().subscribe();
    http.expectOne({ method: 'POST', url: `${GW}/api/auth/logout` }).flush(null, { status: 204, statusText: 'No Content' });

    expect(tokens.get()).toBeNull();
    expect(router.navigate).toHaveBeenCalledWith(['/prijava']);
  });

  it('brise token i kad poziv ne uspe', () => {
    TestBed.inject(AuthService).logout().subscribe({ error: () => undefined });
    http.expectOne(`${GW}/api/auth/logout`).error(new ProgressEvent('error'), { status: 0 });

    expect(tokens.get()).toBeNull();
    expect(router.navigate).toHaveBeenCalledWith(['/prijava']);
  });
});
