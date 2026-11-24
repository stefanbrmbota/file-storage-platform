import { Component } from '@angular/core';
import { RouterLink } from '@angular/router';

/** Stranica za adresu koja ne postoji (ruta '**', T-32.2). */
@Component({
  selector: 'app-not-found',
  imports: [RouterLink],
  template: `
    <h1>404</h1>
    <p>Stranica ne postoji.</p>
    <a routerLink="/">Nazad na početnu</a>
  `,
})
export class NotFound {}
