import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { DatePipe } from '@angular/common';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Observable } from 'rxjs';
import { apiErrorMessage } from '../../core/http/api-error';
import { FileItem, FilesApi, Folder, FolderContents, formatSize, validateName } from './files-api';

/** Šta se trenutno preimenuje (folder ili datoteka) i njegov id. */
interface RenameTarget {
  kind: 'folder' | 'file';
  id: string;
}

/**
 * Stranica Folderi (SB): pregled sadržaja foldera sa putanjom (US-03),
 * novi folder (US-01), preimenovanje (US-11), otpremanje datoteke (US-02)
 * i otpremanje nove verzije (US-10).
 *
 * Adresa: /folderi za korenski folder, /folderi/:id za bilo koji drugi.
 */
@Component({
  selector: 'app-folderi',
  imports: [RouterLink, DatePipe],
  templateUrl: './folderi.html',
  styleUrl: './folderi.css',
})
export class Folderi implements OnInit {
  private readonly api = inject(FilesApi);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly contents = signal<FolderContents | null>(null);
  protected readonly loading = signal(true);
  /** Greška učitavanja stranice (npr. folder ne postoji). */
  protected readonly loadError = signal<string | null>(null);
  /** Greška poslednje akcije (npr. naziv zauzet); stranica ostaje prikazana. */
  protected readonly actionError = signal<string | null>(null);
  protected readonly busy = signal(false);

  protected readonly newFolderName = signal('');
  protected readonly renaming = signal<RenameTarget | null>(null);
  protected readonly renameValue = signal('');

  protected readonly formatSize = formatSize;

  ngOnInit(): void {
    // Isti komponent prikazuje svaki folder: kad se promeni :id u adresi,
    // učitaj novi sadržaj.
    this.route.paramMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
      this.load(params.get('id') ?? 'root');
    });
  }

  private load(folderId: string): void {
    this.loading.set(true);
    this.loadError.set(null);
    this.actionError.set(null);
    this.renaming.set(null);
    this.api.contents(folderId).subscribe({
      next: (c) => {
        this.contents.set(c);
        this.loading.set(false);
      },
      error: (err) => {
        this.contents.set(null);
        this.loadError.set(apiErrorMessage(err));
        this.loading.set(false);
      },
    });
  }

  private reload(): void {
    const c = this.contents();
    this.load(c && c.folder.parentId ? c.folder.id : 'root');
  }

  /** Link za korak putanje: koren je /folderi, ostali /folderi/:id. */
  protected linkTo(index: number, id: string): string[] {
    return index === 0 ? ['/folderi'] : ['/folderi', id];
  }

  protected openFolder(folder: Folder): void {
    this.router.navigate(['/folderi', folder.id]);
  }

  // ---- Novi folder (US-01) ----

  protected createFolder(): void {
    const c = this.contents();
    if (!c) return;
    const name = this.newFolderName();
    const invalid = validateName(name);
    if (invalid) {
      this.actionError.set(invalid);
      return;
    }
    this.run(this.api.createFolder(name.trim(), c.folder.id), () => this.newFolderName.set(''));
  }

  // ---- Preimenovanje (US-11) ----

  protected startRename(kind: 'folder' | 'file', id: string, currentName: string): void {
    this.actionError.set(null);
    this.renaming.set({ kind, id });
    this.renameValue.set(currentName);
  }

  protected cancelRename(): void {
    this.renaming.set(null);
  }

  protected isRenaming(kind: 'folder' | 'file', id: string): boolean {
    const r = this.renaming();
    return r !== null && r.kind === kind && r.id === id;
  }

  protected saveRename(): void {
    const target = this.renaming();
    if (!target) return;
    const name = this.renameValue();
    const invalid = validateName(name);
    if (invalid) {
      this.actionError.set(invalid);
      return;
    }
    const request: Observable<unknown> =
      target.kind === 'folder'
        ? this.api.renameFolder(target.id, name.trim())
        : this.api.renameFile(target.id, name.trim());
    this.run(request, () => this.renaming.set(null));
  }

  // ---- Otpremanje (US-02) i nova verzija (US-10) ----

  protected uploadSelected(event: Event): void {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    const c = this.contents();
    input.value = ''; // isti fajl može ponovo da se izabere
    if (!file || !c) return;
    const invalid = validateName(file.name);
    if (invalid) {
      this.actionError.set(invalid);
      return;
    }
    this.run(this.api.upload(c.folder.id, file));
  }

  protected newVersionSelected(event: Event, target: FileItem): void {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;
    this.run(this.api.uploadNewVersion(target.id, file));
  }

  /** Pokreće akciju; posle uspeha osveži sadržaj, a grešku prikaži iznad liste. */
  private run(request: Observable<unknown>, onSuccess?: () => void): void {
    this.busy.set(true);
    this.actionError.set(null);
    request.subscribe({
      next: () => {
        this.busy.set(false);
        onSuccess?.();
        this.reload();
      },
      error: (err) => {
        this.busy.set(false);
        this.actionError.set(apiErrorMessage(err));
      },
    });
  }
}
