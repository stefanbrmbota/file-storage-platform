import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { Login } from './login';
import { ConfigService } from '../../core/config.service';
import { TokenService } from '../../core/auth/token.service';
import { fakeJwt } from '../../../testing/jwt';

const GW = 'http://gw.test';

describe('Login (prijava u dva koraka)', () => {
  let http: HttpTestingController;
  let router: Router;

  function setup(returnUrl: string | null = null) {
    sessionStorage.clear();
    TestBed.configureTestingModule({
      imports: [Login],
      providers: [
        provideRouter([]),
        provideHttpClient(),
        provideHttpClientTesting(),
        {
          provide: ActivatedRoute,
          useValue: { snapshot: { queryParamMap: convertToParamMap(returnUrl ? { returnUrl } : {}) } },
        },
      ],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    router = TestBed.inject(Router);
    vi.spyOn(router, 'navigateByUrl').mockResolvedValue(true);
    const fixture = TestBed.createComponent(Login);
    fixture.detectChanges();
    return fixture;
  }

  afterEach(() => http.verify());

  function type(el: HTMLElement, selector: string, value: string) {
    const input = el.querySelector<HTMLInputElement>(selector)!;
    input.value = value;
    input.dispatchEvent(new Event('input'));
  }

  async function submit(fixture: ReturnType<typeof setup>) {
    (fixture.nativeElement as HTMLElement).querySelector<HTMLFormElement>('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
    await fixture.whenStable();
  }

  async function passStepOne(fixture: ReturnType<typeof setup>) {
    const el = fixture.nativeElement as HTMLElement;
    type(el, 'input[type=email]', 'ana@example.com');
    type(el, 'input[type=password]', 'Tajna-lozinka1');
    await submit(fixture);
    const req = http.expectOne(`${GW}/api/auth/login`);
    expect(req.request.body).toEqual({ email: 'ana@example.com', password: 'Tajna-lozinka1' });
    req.flush({ message: 'kod za prijavu je poslat na vaš mejl' });
    fixture.detectChanges();
  }

  it('lozinka pa kod: cuva token i vodi na pocetnu', async () => {
    const fixture = setup();
    await passStepOne(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Kod za prijavu je poslat na ana@example.com');
    expect(el.querySelector('input[type=password]')).toBeNull(); // drugi korak

    const token = fakeJwt({ sub: 'u1', role: 'USER', exp: Math.floor(Date.now() / 1000) + 900 });
    type(el, 'input[autocomplete=one-time-code]', '123456');
    await submit(fixture);
    const req = http.expectOne(`${GW}/api/auth/login/verify`);
    expect(req.request.body).toEqual({ email: 'ana@example.com', code: '123456' });
    req.flush({ token });

    expect(TestBed.inject(TokenService).get()).toBe(token);
    expect(router.navigateByUrl).toHaveBeenCalledWith('/');
  });

  it('posle prijave vraca na returnUrl', async () => {
    const fixture = setup('/obavestenja');
    await passStepOne(fixture);
    type(fixture.nativeElement, 'input[autocomplete=one-time-code]', '123456');
    await submit(fixture);
    http.expectOne(`${GW}/api/auth/login/verify`).flush({
      token: fakeJwt({ sub: 'u1', role: 'USER', exp: Math.floor(Date.now() / 1000) + 900 }),
    });
    expect(router.navigateByUrl).toHaveBeenCalledWith('/obavestenja');
  });

  it('returnUrl ka drugom sajtu se ignorise', async () => {
    const fixture = setup('//zlonamerni.example/stranica');
    await passStepOne(fixture);
    type(fixture.nativeElement, 'input[autocomplete=one-time-code]', '123456');
    await submit(fixture);
    http.expectOne(`${GW}/api/auth/login/verify`).flush({
      token: fakeJwt({ sub: 'u1', role: 'USER', exp: Math.floor(Date.now() / 1000) + 900 }),
    });
    expect(router.navigateByUrl).toHaveBeenCalledWith('/');
  });

  it('pogresna lozinka prikazuje poruku servisa i ostaje na prvom koraku', async () => {
    const fixture = setup();
    const el = fixture.nativeElement as HTMLElement;
    type(el, 'input[type=email]', 'ana@example.com');
    type(el, 'input[type=password]', 'pogresna');
    await submit(fixture);
    http.expectOne(`${GW}/api/auth/login`).flush({ error: 'pogrešan email ili lozinka' }, { status: 401, statusText: 'x' });
    fixture.detectChanges();

    expect(el.querySelector('.login__error')?.textContent).toContain('pogrešan email ili lozinka');
    expect(el.querySelector('input[type=password]')).not.toBeNull();
  });

  it('pogresan kod prikazuje poruku i ne cuva token', async () => {
    const fixture = setup();
    await passStepOne(fixture);
    const el = fixture.nativeElement as HTMLElement;
    type(el, 'input[autocomplete=one-time-code]', '000000');
    await submit(fixture);
    http.expectOne(`${GW}/api/auth/login/verify`).flush({ error: 'neispravan ili istekao kod' }, { status: 401, statusText: 'x' });
    fixture.detectChanges();

    expect(el.querySelector('.login__error')?.textContent).toContain('neispravan ili istekao kod');
    expect(TestBed.inject(TokenService).get()).toBeNull();
  });

  it('neispravan unos se ne salje', async () => {
    const fixture = setup();
    type(fixture.nativeElement, 'input[type=email]', 'nije-email');
    await submit(fixture);
    http.expectNone(`${GW}/api/auth/login`);
  });
});
