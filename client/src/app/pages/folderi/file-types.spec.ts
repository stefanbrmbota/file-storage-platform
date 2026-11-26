import { ACCEPT_ATTR, extensionOf, validateFileType, validateNewVersionType } from './file-types';

describe('file-types (US-65)', () => {
  it('prihvata samo ekstenzije sa whitelist-e', () => {
    expect(validateFileType('cv.pdf')).toBeNull();
    expect(validateFileType('SLIKA.PNG')).toBeNull();
    expect(validateFileType('skripta.exe')).not.toBeNull();
    expect(validateFileType('stranica.html')).not.toBeNull();
    expect(validateFileType('crtez.svg')).not.toBeNull();
    expect(validateFileType('README')).not.toBeNull();
    expect(validateFileType('.pdf')).not.toBeNull();
  });

  it('nova verzija mora imati istu ekstenziju', () => {
    expect(validateNewVersionType('cv.pdf', 'cv-v2.PDF')).toBeNull();
    expect(validateNewVersionType('cv.pdf', 'cv.docx')).not.toBeNull();
    expect(validateNewVersionType('cv.pdf', 'cv.exe')).not.toBeNull();
  });

  it('čita ekstenziju i pravi accept atribut', () => {
    expect(extensionOf('a.tar.zip')).toBe('zip');
    expect(extensionOf('bez')).toBe('');
    expect(ACCEPT_ATTR).toContain('.pdf');
    expect(ACCEPT_ATTR).not.toContain('.html');
  });
});
