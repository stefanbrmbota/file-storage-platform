package service

import (
	"errors"

	"github.com/google/uuid"
)

// ErrInvalidID znači da id foldera ili datoteke nije u ispravnom obliku (US-64).
var ErrInvalidID = errors.New("invalid id")

// forbiddenNameChars su specijalni karakteri koji nisu dozvoljeni u nazivu
// foldera ili datoteke (pored / i \), jer ih ne prihvataju uobičajeni
// fajl sistemi i često se koriste u napadima na putanje i prikaz.
const forbiddenNameChars = `<>:"|?*`

// validateID prihvata samo UUID u kanonskom obliku (36 znakova), kakve
// servis i pravi. Sve ostalo se odbija pre nego što stigne do baze.
func validateID(id string) error {
	if len(id) != 36 {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(id); err != nil {
		return ErrInvalidID
	}
	return nil
}
