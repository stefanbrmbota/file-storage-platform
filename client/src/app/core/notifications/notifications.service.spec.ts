import { TestBed } from '@angular/core/testing';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { NotificationsService } from './notifications.service';
import { ConfigService } from '../config.service';
import { ErrorBannerService } from '../notify/error-banner.service';
import { authInterceptor } from '../http/auth.interceptor';

const GW = 'http://gw.test';

describe('NotificationsService', () => {
  let http: HttpTestingController;
  let s: NotificationsService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideRouter([]), provideHttpClient(withInterceptors([authInterceptor])), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: GW });
    http = TestBed.inject(HttpTestingController);
    s = TestBed.inject(NotificationsService);
  });

  afterEach(() => http.verify());

  it('list salje limit i kursor, bez ID-ja korisnika', () => {
    s.list('abc', 10).subscribe();
    const req = http.expectOne((r) => r.url === `${GW}/api/notifications`);
    expect(req.request.params.get('limit')).toBe('10');
    expect(req.request.params.get('cursor')).toBe('abc');
    expect(req.request.params.has('userId')).toBe(false);
    req.flush({ items: [] });
  });

  it('refreshUnread postavlja broj, a reset ga brise', () => {
    s.refreshUnread();
    http.expectOne(`${GW}/api/notifications/unread-count`).flush({ count: 5 });
    expect(s.unreadCount()).toBe(5);
    s.reset();
    expect(s.unreadCount()).toBe(0);
  });

  it('greska pri broju neprocitanih ne prikazuje traku i ne menja broj', () => {
    s.refreshUnread();
    http.expectOne(`${GW}/api/notifications/unread-count`).flush({ count: 3 });
    s.refreshUnread();
    http.expectOne(`${GW}/api/notifications/unread-count`).flush({ error: 'x' }, { status: 503, statusText: 'x' });

    expect(s.unreadCount()).toBe(3);
    expect(TestBed.inject(ErrorBannerService).message()).toBeNull();
  });
});
