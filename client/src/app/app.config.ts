import {
  ApplicationConfig,
  inject,
  provideAppInitializer,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import { provideHttpClient, withFetch, withInterceptors } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { routes } from './app.routes';
import { ConfigService } from './core/config.service';
import { authInterceptor } from './core/http/auth.interceptor';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes),
    // Token uz svaki zahtev ka gateway-u, 401 -> prijava, greske servera u traku (T-32.3).
    provideHttpClient(withFetch(), withInterceptors([authInterceptor])),
    // Adresa gateway-a se ucitava iz /config.json pre prvog prikaza.
    provideAppInitializer(() => inject(ConfigService).load()),
  ],
};
