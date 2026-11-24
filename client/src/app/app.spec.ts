import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { App } from './app';
import { ConfigService } from './core/config.service';

describe('App', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: 'http://gw.test' });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('poziva /health na adresi gateway-a iz konfiguracije', async () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();

    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
    await fixture.whenStable();

    const status = (fixture.nativeElement as HTMLElement).querySelector('.status');
    expect(status?.textContent).toContain('dostupan');
  });

  it('prikazuje da gateway nije dostupan kad poziv ne uspe', async () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();

    http.expectOne('http://gw.test/health').flush(null, { status: 503, statusText: 'Service Unavailable' });
    await fixture.whenStable();

    const status = (fixture.nativeElement as HTMLElement).querySelector('.status');
    expect(status?.textContent).toContain('nedostupan');
  });
});
