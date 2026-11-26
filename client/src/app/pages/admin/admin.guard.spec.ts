import { TestBed } from '@angular/core/testing';
import { ActivatedRouteSnapshot, Router, RouterStateSnapshot, UrlTree, provideRouter } from '@angular/router';
import { adminGuard } from './admin.guard';
import { TokenService } from '../../core/auth/token.service';
import { fakeJwt } from '../../../testing/jwt';

describe('adminGuard', () => {
  function run(): boolean | UrlTree {
    return TestBed.runInInjectionContext(() => adminGuard({} as ActivatedRouteSnapshot, {} as RouterStateSnapshot)) as boolean | UrlTree;
  }

  function loginAs(role: string | null) {
    sessionStorage.clear();
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    if (role !== null) {
      TestBed.inject(TokenService).set(fakeJwt({ sub: 'u1', role, exp: Math.floor(Date.now() / 1000) + 900 }));
    }
  }

  it('propusta administratora', () => {
    loginAs('ADMIN');
    expect(run()).toBe(true);
  });

  it('običan korisnik se vraća na početnu', () => {
    loginAs('USER');
    const result = run();
    expect(result).toBeInstanceOf(UrlTree);
    expect(TestBed.inject(Router).serializeUrl(result as UrlTree)).toBe('/');
  });

  it('bez tokena ili sa nepoznatom ulogom se ne propušta', () => {
    loginAs(null);
    expect(run()).not.toBe(true);

    TestBed.resetTestingModule();
    loginAs('');
    expect(run()).not.toBe(true);
  });
});
