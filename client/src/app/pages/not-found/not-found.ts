import { Component } from '@angular/core';
import { RouterLink } from '@angular/router';

/** Stranica za adresu koja ne postoji (ruta '**', T-32.2). */
@Component({
  selector: 'app-not-found',
  imports: [RouterLink],
  template: `
    <section class="nf">
      <h1>404</h1>
      <p>Stranica ne postoji.</p>
      <a routerLink="/">Nazad na početnu</a>
    </section>
  `,
  styles: `
    .nf {
      margin: 64px auto;
      text-align: center;
    }
    h1 {
      margin-bottom: 0.25rem;
      color: var(--accent);
      font-size: 4rem;
      font-weight: 700;
      letter-spacing: -0.04em;
    }
    p {
      color: var(--muted);
    }
  `,
})
export class NotFound {}
