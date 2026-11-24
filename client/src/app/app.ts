import { Component } from '@angular/core';
import { RouterOutlet } from '@angular/router';

/** Koren aplikacije: sve prikazuje ruter (raspored i stranice su u app.routes.ts). */
@Component({
  imports: [RouterOutlet],
  selector: 'app-root',
  template: '<router-outlet />',
})
export class App {}
