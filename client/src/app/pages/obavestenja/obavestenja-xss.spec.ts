import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Obavestenja } from './obavestenja';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test';
const PAYLOAD = '<img src=x onerror="window.__xss = true"><script>window.__xss = true</script>';

/** US-66: tekst obaveštenja (može sadržati ime korisnika ili naziv fajla) je samo tekst. */
describe('Obavestenja: enkodovanje izlaza (US-66)', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [Obavestenja],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    delete (window as unknown as Record<string, unknown>)['__xss'];
  });

  afterEach(() => http.verify());

  it('poruka obaveštenja se prikazuje kao tekst, ne kao HTML', async () => {
    const fixture = TestBed.createComponent(Obavestenja);
    fixture.detectChanges();
    http.expectOne(`${GW}/api/notifications/unread-count`).flush({ count: 1 });
    http
      .expectOne((r) => r.url === `${GW}/api/notifications`)
      .flush({ items: [{ id: '1', type: PAYLOAD, message: PAYLOAD, isRead: false, createdAt: '2026-11-24T08:00:00Z' }] });
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    expect(el.querySelector('.item__message')?.textContent?.trim()).toBe(PAYLOAD);
    expect(el.querySelector('img, script')).toBeNull();
    expect((window as unknown as Record<string, unknown>)['__xss']).toBeUndefined();
  });
});
