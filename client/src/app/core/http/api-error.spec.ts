import { HttpErrorResponse } from '@angular/common/http';
import { apiErrorMessage } from './api-error';

const err = (status: number, error: unknown = null) => new HttpErrorResponse({ status, error });

describe('apiErrorMessage', () => {
  it('koristi poruku servisa iz {"error": "..."}', () => {
    expect(apiErrorMessage(err(409, { error: 'folder sa tim imenom vec postoji' }))).toBe('folder sa tim imenom vec postoji');
  });

  it('daje opstu poruku kad servis nije poslao poruku', () => {
    expect(apiErrorMessage(err(403))).toContain('pravo');
    expect(apiErrorMessage(err(404, 'tekst'))).toContain('ne postoji');
    expect(apiErrorMessage(err(503))).toContain('Server');
  });

  it('prepoznaje da server nije dostupan (status 0)', () => {
    expect(apiErrorMessage(err(0))).toContain('nije dostupan');
  });

  it('radi i za greske koje nisu HTTP', () => {
    expect(apiErrorMessage(new Error('x'))).toContain('greške');
  });
});
