import { Component } from '@angular/core';

/**
 * Stranica za prijavu. Za sada samo mesto na koje interceptor vodi korisnika
 * kad mu token istekne (T-32.3); formu u dva koraka (lozinka, pa OTP) dodaje T-32.4.
 */
@Component({
  selector: 'app-login',
  template: `
    <h1>Prijava</h1>
    <p>Prijavi se da nastaviš.</p>
  `,
})
export class Login {}
