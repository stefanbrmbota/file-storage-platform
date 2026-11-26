import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Folderi } from './folderi';
import { FolderContents } from './files-api';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test/api';
const SCRIPT = '<script>window.__xss = true</script>';
const IMG = '<img src=x onerror="window.__xss = true">';

/**
 * US-66: korisnički podaci (nazivi foldera i datoteka, tip, poruke greške)
 * prikazuju se samo kao tekst. Angular interpolacija {{ }} ih enkoduje, pa
 * HTML iz naziva nikad ne postaje element na stranici.
 */
describe('Folderi: enkodovanje izlaza (US-66)', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [Folderi],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: 'http://gw.test' });
    http = TestBed.inject(HttpTestingController);
    delete (window as unknown as Record<string, unknown>)['__xss'];
  });

  afterEach(() => http.verify());

  function contents(): FolderContents {
    return {
      folder: { id: 'r1', name: 'root', ownerId: 'u1', createdAt: '', updatedAt: '' },
      path: [{ id: 'r1', name: 'root' }],
      folders: [{ id: 'f1', name: IMG, ownerId: 'u1', parentId: 'r1', createdAt: '', updatedAt: '' }],
      files: [
        {
          id: 'd1', name: SCRIPT, contentType: '<b>text/html</b>', size: 1,
          ownerId: 'u1', parentId: 'r1', createdAt: '', updatedAt: '',
        },
      ],
    };
  }

  it('nazivi i tip datoteke se prikazuju kao tekst, ne kao HTML', async () => {
    const fixture = TestBed.createComponent(Folderi);
    fixture.detectChanges();
    http.expectOne(`${GW}/folders/root/contents`).flush(contents());
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    expect(el.textContent).toContain(SCRIPT);
    expect(el.textContent).toContain(IMG);
    expect(el.textContent).toContain('<b>text/html</b>');
    expect(el.querySelector('table script, table img, table b')).toBeNull();
    expect((window as unknown as Record<string, unknown>)['__xss']).toBeUndefined();
  });

  it('poruka greške sa servera se prikazuje kao tekst', async () => {
    const fixture = TestBed.createComponent(Folderi);
    fixture.detectChanges();
    http.expectOne(`${GW}/folders/root/contents`).flush({ error: IMG }, { status: 404, statusText: 'Not Found' });
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    expect(el.querySelector('[role="alert"]')?.textContent).toContain(IMG);
    expect(el.querySelector('img')).toBeNull();
  });
});
