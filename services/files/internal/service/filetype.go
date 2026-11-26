package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// ErrFileTypeNotAllowed znači da ekstenzija nije na whitelisti ili da se
// sadržaj datoteke ne slaže sa ekstenzijom (US-65).
var ErrFileTypeNotAllowed = errors.New("file type is not allowed")

// sniffLen je broj bajtova koje http.DetectContentType gleda.
const sniffLen = 512

// fileType opisuje jedan dozvoljeni tip: Content-Type koji servis čuva i
// tipove koje http.DetectContentType može da prepozna iz sadržaja.
type fileType struct {
	contentType string
	sniffed     []string
}

// knownTypes su tipovi koje servis ume da proveri po sadržaju. HTML, SVG,
// JavaScript i izvršne datoteke namerno nisu tu: mogu da nose kod (XSS).
// Office formati (docx, xlsx, pptx) su ZIP arhive, pa se tako i prepoznaju.
var knownTypes = map[string]fileType{
	".pdf":  {"application/pdf", []string{"application/pdf"}},
	".png":  {"image/png", []string{"image/png"}},
	".jpg":  {"image/jpeg", []string{"image/jpeg"}},
	".jpeg": {"image/jpeg", []string{"image/jpeg"}},
	".gif":  {"image/gif", []string{"image/gif"}},
	".webp": {"image/webp", []string{"image/webp"}},
	".txt":  {"text/plain; charset=utf-8", []string{"text/plain"}},
	".csv":  {"text/csv; charset=utf-8", []string{"text/plain"}},
	".md":   {"text/markdown; charset=utf-8", []string{"text/plain"}},
	".json": {"application/json", []string{"text/plain"}},
	".zip":  {"application/zip", []string{"application/zip"}},
	".docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", []string{"application/zip"}},
	".xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", []string{"application/zip"}},
	".pptx": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", []string{"application/zip"}},
	".mp3":  {"audio/mpeg", []string{"audio/mpeg"}},
	".mp4":  {"video/mp4", []string{"video/mp4"}},
}

// FileTypePolicy je whitelist ekstenzija koje korisnici smeju da otpreme.
type FileTypePolicy struct {
	allowed map[string]fileType
}

// NewFileTypePolicy pravi whitelist od liste ekstenzija (sa tačkom ili bez,
// npr. "pdf" ili ".pdf"). Prazna lista znači sve poznate tipove. Ekstenzija
// koju servis ne ume da proveri po sadržaju se ignoriše.
func NewFileTypePolicy(extensions []string) *FileTypePolicy {
	p := &FileTypePolicy{allowed: map[string]fileType{}}
	for _, ext := range extensions {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if t, ok := knownTypes[ext]; ok {
			p.allowed[ext] = t
		}
	}
	if len(p.allowed) == 0 {
		for ext, t := range knownTypes {
			p.allowed[ext] = t
		}
	}
	return p
}

// Check proverava naziv i sadržaj datoteke. Vraća Content-Type koji se čuva
// (određen po proverenom tipu, a ne po zaglavlju koje je poslao klijent) i
// reader koji ponovo daje ceo sadržaj, uključujući pročitani početak.
func (p *FileTypePolicy) Check(name string, content io.Reader) (string, io.Reader, error) {
	t, ok := p.allowed[strings.ToLower(filepath.Ext(name))]
	if !ok {
		return "", nil, ErrFileTypeNotAllowed
	}

	head := make([]byte, sniffLen)
	n, err := io.ReadFull(content, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", nil, err
	}
	head = head[:n]

	sniffed, _, _ := strings.Cut(http.DetectContentType(head), ";")
	for _, want := range t.sniffed {
		if sniffed == want {
			return t.contentType, io.MultiReader(bytes.NewReader(head), content), nil
		}
	}
	return "", nil, ErrFileTypeNotAllowed
}
