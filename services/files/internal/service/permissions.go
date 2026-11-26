package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/stefanbrmbota/file-storage-platform/services/files/internal/model"
)

// canWrite odlučuje da li korisnik sme da upisuje sadržaj u folder (US-65).
// Za sada pravo upisa ima samo vlasnik; deljenje sa pravom upisa dolazi sa
// Sharing servisom i tada se proverava ovde, na jednom mestu.
func canWrite(userID, ownerID string) bool {
	return userID != "" && userID == ownerID
}

// uploadTarget vraća folder u koji korisnik otprema datoteku i proverava da
// li sme da piše u njega. Tuđi folder se klijentu prijavljuje kao nepostojeći
// (ne otkriva se da postoji), ali se pokušaj beleži u log.
func (s *FolderService) uploadTarget(ctx context.Context, userID, folderID string) (*model.Folder, error) {
	folder, err := s.ownFolder(ctx, userID, folderID)
	if errors.Is(err, ErrFolderNotFound) && folderID != "" && folderID != RootFolderID {
		// Folder postoji, ali je tuđi: klijent dobija isto što i za
		// nepostojeći, a pokušaj upisa u tuđi folder ide u log.
		if f, ferr := s.store.FindByID(ctx, folderID); ferr == nil && !canWrite(userID, f.OwnerID) {
			slog.Warn("otpremanje odbijeno: korisnik nema pravo upisa u folder",
				"userId", userID, "folderId", f.ID)
		}
	}
	return folder, err
}

// checkFileWrite proverava da li korisnik sme da otpremi novu verziju
// datoteke. Kao i za folder, tuđa datoteka izgleda kao nepostojeća.
func checkFileWrite(userID string, file *model.File) error {
	if !canWrite(userID, file.OwnerID) {
		slog.Warn("nova verzija odbijena: korisnik nema pravo upisa u datoteku",
			"userId", userID, "fileId", file.ID)
		return ErrFileNotFound
	}
	return nil
}
