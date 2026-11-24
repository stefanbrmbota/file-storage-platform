import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Primer } from './primer';
import { ConfigService } from '../../core/config.service';

// Primer testa za stranicu: lazni gateway (HttpTestingController) umesto pravog.
describe('Primer', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [Primer],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: 'http://gw.test' });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  async function render(flush: (req: ReturnType<HttpTestingController['expectOne']>) => void) {
    const fixture = TestBed.createComponent(Primer);
    fixture.detectChanges();
    flush(http.expectOne('http://gw.test/api/auth/session'));
    await fixture.whenStable();
    return (fixture.nativeElement as HTMLElement).textContent ?? '';
  }

  it('prikazuje podatke sa gateway-a', async () => {
    const text = await render((req) => req.flush({ userId: 'u1', role: 'USER' }));
    expect(text).toContain('uloga: USER');
  });

  it('prikazuje poruku greske iz {"error": "..."}', async () => {
    const text = await render((req) => req.flush({ error: 'servis trenutno nije dostupan' }, { status: 503, statusText: 'x' }));
    expect(text).toContain('servis trenutno nije dostupan');
  });
});
