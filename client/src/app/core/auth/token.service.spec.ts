import { TestBed } from '@angular/core/testing';
import { TokenService, decodeClaims } from './token.service';
import { fakeJwt } from '../../../testing/jwt';

const inMinutes = (m: number) => Math.floor(Date.now() / 1000) + m * 60;

describe('TokenService', () => {
  beforeEach(() => sessionStorage.clear());

  function create(): TokenService {
    TestBed.resetTestingModule();
    return TestBed.inject(TokenService);
  }

  it('cuva token, cita claims i pamti ga posle osvezavanja', () => {
    const t = fakeJwt({ sub: 'u1', role: 'USER', exp: inMinutes(15) });
    const s = create();
    s.set(t);

    expect(s.get()).toBe(t);
    expect(s.isAuthenticated()).toBe(true);
    expect(s.claims()?.userId).toBe('u1');
    expect(s.claims()?.role).toBe('USER');

    // Novi servis (kao posle F5) cita token iz sessionStorage.
    expect(create().get()).toBe(t);
  });

  it('clear brise token i iz memorije i iz sessionStorage', () => {
    const s = create();
    s.set(fakeJwt({ sub: 'u1', role: 'USER', exp: inMinutes(15) }));
    s.clear();
    expect(s.get()).toBeNull();
    expect(s.isAuthenticated()).toBe(false);
    expect(sessionStorage.length).toBe(0);
  });

  it('istekao token se ne cuva i ne vraca', () => {
    const s = create();
    s.set(fakeJwt({ sub: 'u1', role: 'USER', exp: inMinutes(-1) }));
    expect(s.get()).toBeNull();

    sessionStorage.setItem('fsp.token', fakeJwt({ sub: 'u1', role: 'USER', exp: inMinutes(-1) }));
    expect(create().get()).toBeNull();
    expect(sessionStorage.length).toBe(0);
  });

  it('neispravan token se odbacuje', () => {
    const s = create();
    s.set('nije.jwt');
    expect(s.get()).toBeNull();
    expect(decodeClaims('abc')).toBeNull();
    expect(decodeClaims(fakeJwt({ role: 'USER', exp: inMinutes(5) }))).toBeNull(); // bez sub
  });
});
