import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { ConfigService } from '../../core/config.service';

/** Folder kako ga vraća Files servis. */
export interface Folder {
  id: string;
  name: string;
  ownerId: string;
  parentId?: string;
  createdAt: string;
  updatedAt: string;
}

/** Metapodaci datoteke kako ih vraća Files servis (sadržaj je u MinIO-u). */
export interface FileItem {
  id: string;
  name: string;
  contentType: string;
  size: number;
  ownerId: string;
  parentId: string;
  createdAt: string;
  updatedAt: string;
}

/** Jedan korak putanje od korena do trenutnog foldera. */
export interface PathItem {
  id: string;
  name: string;
}

/** Odgovor GET /api/folders/{id}/contents (US-03). */
export interface FolderContents {
  folder: Folder;
  path: PathItem[];
  folders: Folder[];
  files: FileItem[];
}

/**
 * Pozivi Files servisa kroz gateway (SB). Token dodaje interceptor,
 * adresa gateway-a dolazi iz config.json.
 */
@Injectable({ providedIn: 'root' })
export class FilesApi {
  private readonly http = inject(HttpClient);
  private readonly config = inject(ConfigService);

  private url(path: string): string {
    return `${this.config.gatewayUrl}/api${path}`;
  }

  /** Sadržaj foldera; "root" je korenski folder prijavljenog korisnika. */
  contents(folderId: string): Observable<FolderContents> {
    return this.http.get<FolderContents>(this.url(`/folders/${encodeURIComponent(folderId)}/contents`));
  }

  createFolder(name: string, parentId: string): Observable<Folder> {
    return this.http.post<Folder>(this.url('/folders'), { name, parentId });
  }

  renameFolder(id: string, name: string): Observable<Folder> {
    return this.http.patch<Folder>(this.url(`/folders/${encodeURIComponent(id)}`), { name });
  }

  renameFile(id: string, name: string): Observable<FileItem> {
    return this.http.patch<FileItem>(this.url(`/files/${encodeURIComponent(id)}`), { name });
  }

  /** Otpremanje nove datoteke (US-02), multipart polje "file". */
  upload(folderId: string, file: File): Observable<FileItem> {
    return this.http.post<FileItem>(this.url(`/folders/${encodeURIComponent(folderId)}/files`), formWith(file));
  }

  /** Nova verzija postojeće datoteke (US-10); naziv ostaje isti. */
  uploadNewVersion(fileId: string, file: File): Observable<FileItem> {
    return this.http.put<FileItem>(this.url(`/files/${encodeURIComponent(fileId)}/content`), formWith(file));
  }
}

function formWith(file: File): FormData {
  const form = new FormData();
  form.append('file', file, file.name);
  return form;
}

/**
 * Ista pravila za naziv kao na serveru (validacija i na klijentu, IB 2.18):
 * obavezan, najviše 255 znakova, bez / i \ i bez imena "." i "..".
 * Vraća poruku greške ili null ako je naziv ispravan.
 */
export function validateName(raw: string): string | null {
  const name = raw.trim();
  if (name === '') return 'Naziv je obavezan.';
  if (name === '.' || name === '..') return 'Naziv ne može biti "." ili "..".';
  if ([...name].length > 255) return 'Naziv može imati najviše 255 znakova.';
  if (/[/\\]/.test(name)) return 'Naziv ne sme da sadrži / ni \\.';
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f]/.test(name)) return 'Naziv ne sme da sadrži kontrolne znakove.';
  return null;
}

/** Veličina u čitljivom obliku: 1536 → "1.5 KB". */
export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB'];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(1)} ${units[unit]}`;
}
