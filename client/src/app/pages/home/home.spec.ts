import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Home } from './home';
import { ConfigService } from '../../core/config.service';

describe('Home', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [Home],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: 'http://gw.test' });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('prikazuje da gateway nije dostupan kad /health ne uspe', async () => {
    const fixture = TestBed.createComponent(Home);
    fixture.detectChanges();
    http.expectOne('http://gw.test/health').flush(null, { status: 503, statusText: 'Service Unavailable' });
    await fixture.whenStable();

    const status = (fixture.nativeElement as HTMLElement).querySelector('.status');
    expect(status?.textContent).toContain('nedostupan');
  });
});
