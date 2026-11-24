import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { Title } from '@angular/platform-browser';
import { routes } from './app.routes';
import { ConfigService } from './core/config.service';

describe('Rutiranje (T-32.2)', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideRouter(routes), provideHttpClient(), provideHttpClientTesting()],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: 'http://gw.test' });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  function text(harness: RouterTestingHarness): string {
    return (harness.routeNativeElement?.ownerDocument.body.textContent ?? '').replace(/\s+/g, ' ');
  }

  it('/ prikazuje pocetnu stranicu u rasporedu sa zaglavljem', async () => {
    const harness = await RouterTestingHarness.create('/');
    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
    harness.detectChanges();

    const root = harness.fixture.nativeElement as HTMLElement;
    expect(root.querySelector('header .brand')?.textContent).toContain('File Storage');
    expect(root.querySelector('nav a')?.textContent).toContain('Početna');
    expect(text(harness)).toContain('Dobrodošli');
    expect(text(harness)).toContain('Gateway: dostupan');
    expect(TestBed.inject(Title).getTitle()).toBe('Početna | File Storage');
  });

  it('nepostojeca adresa prikazuje 404, a zaglavlje ostaje', async () => {
    const harness = await RouterTestingHarness.create('/nepostojeca/stranica');
    const root = harness.fixture.nativeElement as HTMLElement;

    expect(root.querySelector('h1')?.textContent).toBe('404');
    expect(root.querySelector('header .brand')).not.toBeNull();
    expect(TestBed.inject(Title).getTitle()).toBe('Stranica ne postoji | File Storage');
  });

  it('link "Nazad na pocetnu" vodi na / bez ponovnog ucitavanja', async () => {
    const harness = await RouterTestingHarness.create('/nema');
    const link = (harness.fixture.nativeElement as HTMLElement).querySelector<HTMLAnchorElement>(
      'main a[href="/"]',
    );
    expect(link).not.toBeNull();

    link!.click(); // ruter preuzima klik, nema novog zahteva za stranicu
    await harness.fixture.whenStable();
    http.expectOne('http://gw.test/health').flush({ status: 'ok' });
    harness.detectChanges();

    expect(TestBed.inject(Router).url).toBe('/');
    expect(text(harness)).toContain('Dobrodošli');
  });
});
