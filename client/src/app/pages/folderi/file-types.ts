/**
 * Whitelist ekstenzija za otpremanje, ista kao podrazumevana lista u Files
 * servisu (FILES_ALLOWED_EXTENSIONS, US-65). Klijent proverava samo
 * ekstenziju; server dodatno proverava i sadržaj datoteke.
 */
export const ALLOWED_EXTENSIONS = [
  'pdf', 'png', 'jpg', 'jpeg', 'gif', 'webp', 'txt', 'csv', 'md', 'json',
  'zip', 'docx', 'xlsx', 'pptx', 'mp3', 'mp4',
] as const;

/** Vrednost za atribut accept na <input type="file">. */
export const ACCEPT_ATTR = ALLOWED_EXTENSIONS.map((e) => '.' + e).join(',');

/** Ekstenzija iz naziva, malim slovima, bez tačke ('' ako je nema). */
export function extensionOf(name: string): string {
  const dot = name.lastIndexOf('.');
  return dot > 0 && dot < name.length - 1 ? name.slice(dot + 1).toLowerCase() : '';
}

/** Vraća poruku greške ako tip datoteke nije dozvoljen, inače null. */
export function validateFileType(name: string): string | null {
  const ext = extensionOf(name);
  if ((ALLOWED_EXTENSIONS as readonly string[]).includes(ext)) return null;
  return `Tip datoteke nije dozvoljen. Dozvoljeno: ${ALLOWED_EXTENSIONS.join(', ')}.`;
}

/**
 * Nova verzija mora imati istu ekstenziju kao postojeća datoteka, jer server
 * proverava sadržaj prema nazivu postojeće datoteke.
 */
export function validateNewVersionType(existingName: string, newName: string): string | null {
  const invalid = validateFileType(newName);
  if (invalid) return invalid;
  if (extensionOf(existingName) !== extensionOf(newName)) {
    return `Nova verzija mora biti istog tipa (.${extensionOf(existingName)}).`;
  }
  return null;
}
