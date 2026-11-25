import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { Folderi } from './folderi';
import { FolderContents, formatSize, validateName } from './files-api';
import { ConfigService } from '../../core/config.service';

const GW = 'http://gw.test/api';

function rootContents(): FolderContents {
  return {
    folder: { id: 'r1', name: 'root', ownerId: 'u1', createdAt: '', updatedAt: '' },
    path: [{ id: 'r1', name: 'root' }],
    folders: [{ id: 'f1', name: 'Fakultet', ownerId: 'u1', parentId: 'r1', createdAt: '', updatedAt: '2026-11-25T10:00:00Z' }],
    files: [
      {
        id: 'd1', name: 'cv.pdf', contentType: 'application/pdf', size: 2048,
        ownerId: 'u1', parentId: 'r1', createdAt: '', updatedAt: '2026-11-25T10:00:00Z',
      },
    ],
  };
}

// Lazni gateway (HttpTestingController) umesto pravog, kao u pages/primer.
describe('Folderi', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [Folderi],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    TestBed.inject(ConfigService).setForTest({ gatewayUrl: 'http://gw.test' });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  async function renderRoot() {
    const fixture = TestBed.createComponent(Folderi);
    fixture.detectChanges();
    http.expectOne(`${GW}/folders/root/contents`).flush(rootContents());
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  it('prikazuje podfoldere i datoteke korenskog foldera', async () => {
    const fixture = await renderRoot();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Fakultet');
    expect(text).toContain('cv.pdf');
    expect(text).toContain('2.0 KB');
    expect(text).toContain('Moji fajlovi');
  });

  it('pravi novi folder u trenutnom folderu i osvežava listu', async () => {
    const fixture = await renderRoot();
    const el = fixture.nativeElement as HTMLElement;
    const input = el.querySelector<HTMLInputElement>('#new-folder-name')!;
    input.value = 'Slike';
    input.dispatchEvent(new Event('input'));
    el.querySelector<HTMLFormElement>('form.new-folder')!.dispatchEvent(new Event('submit'));

    const req = http.expectOne(`${GW}/folders`);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ name: 'Slike', parentId: 'r1' });
    req.flush({ id: 'f2', name: 'Slike' });
    http.expectOne(`${GW}/folders/root/contents`).flush(rootContents());
  });

  it('ne šalje zahtev za neispravan naziv foldera', async () => {
    const fixture = await renderRoot();
    const el = fixture.nativeElement as HTMLElement;
    const input = el.querySelector<HTMLInputElement>('#new-folder-name')!;
    input.value = 'a/b';
    input.dispatchEvent(new Event('input'));
    el.querySelector<HTMLFormElement>('form.new-folder')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();

    http.expectNone(`${GW}/folders`);
    expect(el.textContent).toContain('ne sme da sadrži');
  });

  it('prikazuje poruku servisa kad je naziv zauzet', async () => {
    const fixture = await renderRoot();
    const el = fixture.nativeElement as HTMLElement;
    const input = el.querySelector<HTMLInputElement>('#new-folder-name')!;
    input.value = 'Fakultet';
    input.dispatchEvent(new Event('input'));
    el.querySelector<HTMLFormElement>('form.new-folder')!.dispatchEvent(new Event('submit'));

    http
      .expectOne(`${GW}/folders`)
      .flush({ error: 'an item with this name already exists in the folder' }, { status: 409, statusText: 'Conflict' });
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.textContent).toContain('already exists');
  });

  it('otprema izabrani fajl kao multipart polje "file"', async () => {
    const fixture = await renderRoot();
    const input = (fixture.nativeElement as HTMLElement).querySelector<HTMLInputElement>('.upload input[type=file]')!;
    const file = new File(['zdravo'], 'beleske.txt', { type: 'text/plain' });
    Object.defineProperty(input, 'files', { value: [file] });
    input.dispatchEvent(new Event('change'));

    const req = http.expectOne(`${GW}/folders/r1/files`);
    expect(req.request.method).toBe('POST');
    expect((req.request.body as FormData).get('file')).toBeTruthy();
    req.flush({ id: 'd2', name: 'beleske.txt' });
    http.expectOne(`${GW}/folders/root/contents`).flush(rootContents());
  });

  it('prikazuje grešku kad folder ne postoji', async () => {
    const fixture = TestBed.createComponent(Folderi);
    fixture.detectChanges();
    http.expectOne(`${GW}/folders/root/contents`).flush({ error: 'folder not found' }, { status: 404, statusText: 'Not Found' });
    await fixture.whenStable();
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('folder not found');
  });
});

describe('validateName i formatSize', () => {
  it('proverava naziv kao server', () => {
    expect(validateName('Fakultet')).toBeNull();
    expect(validateName('   ')).not.toBeNull();
    expect(validateName('..')).not.toBeNull();
    expect(validateName('a\\b')).not.toBeNull();
    expect(validateName('x'.repeat(256))).not.toBeNull();
  });

  it('prikazuje veličinu čitljivo', () => {
    expect(formatSize(500)).toBe('500 B');
    expect(formatSize(1536)).toBe('1.5 KB');
    expect(formatSize(5 * 1024 * 1024)).toBe('5.0 MB');
  });
});
