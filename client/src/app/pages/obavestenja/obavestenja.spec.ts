import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Obavestenja } from './obavestenja';
import { ConfigService } from '../../core/config.service';
import { NotificationsService } from '../../core/notifications/notifications.service';

const GW = 'http://gw.test';
const n = (id: string, message: string, createdAt: string, isRead = false) => ({
  id,
  type: 'SHARE_GRANTED',
  message,
  isRead,
  createdAt,
});

describe('Obavestenja (T-08.2)', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [Obavestenja],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  function open() {
    const fixture = TestBed.createComponent(Obavestenja);
    fixture.detectChanges();
    http.expectOne(`${GW}/api/notifications/unread-count`).flush({ count: 2 });
    return fixture;
  }

  const listReq = (cursor?: string) =>
    http.expectOne(
      (r) =>
        r.url === `${GW}/api/notifications` &&
        r.params.get('limit') === '20' &&
        (cursor ? r.params.get('cursor') === cursor : !r.params.has('cursor')),
    );

  const rows = (el: HTMLElement) => Array.from(el.querySelectorAll('.item__message')).map((e) => e.textContent?.trim());

  it('prikazuje obavestenja redom kojim stizu (od najnovijeg), sa tekstom i vremenom', async () => {
    const fixture = open();
    listReq().flush({
      items: [
        n('2', 'Ceca je podelila fajl', '2026-11-24T08:00:00Z'),
        n('1', 'Bojan je podelio folder', '2026-11-20T09:15:00Z', true),
      ],
    });
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;

    expect(rows(el)).toEqual(['Ceca je podelila fajl', 'Bojan je podelio folder']);
    const times = Array.from(el.querySelectorAll('time')).map((t) => t.getAttribute('datetime'));
    expect(times).toEqual(['2026-11-24T08:00:00Z', '2026-11-20T09:15:00Z']);
    expect(el.querySelector('time')?.textContent).toContain('2026');
    // Neprocitano je oznaceno, procitano nije.
    expect(el.querySelectorAll('.item--unread').length).toBe(1);
    expect(el.querySelector('.more')).toBeNull(); // nema sledece stranice
    expect(TestBed.inject(NotificationsService).unreadCount()).toBe(2);
  });

  it('"Ucitaj jos" dodaje sledecu stranicu ispod', async () => {
    const fixture = open();
    listReq().flush({ items: [n('3', 'treca', '2026-11-24T10:00:00Z')], nextCursor: '3' });
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;

    el.querySelector<HTMLButtonElement>('.more')!.click();
    listReq('3').flush({ items: [n('2', 'druga', '2026-11-23T10:00:00Z'), n('1', 'prva', '2026-11-22T10:00:00Z')] });
    await fixture.whenStable();

    expect(rows(el)).toEqual(['treca', 'druga', 'prva']);
    expect(el.querySelector('.more')).toBeNull();
  });

  it('bez obavestenja prikazuje poruku', async () => {
    const fixture = open();
    listReq().flush({ items: [] });
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).querySelector('.empty')?.textContent).toContain('Nemaš obaveštenja');
  });

  it('greska prikazuje poruku servisa', async () => {
    const fixture = open();
    listReq().flush({ error: 'servis trenutno nije dostupan' }, { status: 503, statusText: 'x' });
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).querySelector('.error')?.textContent).toContain(
      'servis trenutno nije dostupan',
    );
  });
});
