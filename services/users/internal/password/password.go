// Package password heshira i proverava lozinke pomocu Argon2id.
//
// Hash vraca jedan string u PHC formatu:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<so b64>$<hash b64>
//
// Parametri, so i hash su u samom stringu, pa Verify ne zavisi od trenutnih
// env podesavanja i stari heshevi ostaju validni kad se parametri pojacaju.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Podrazumevane vrednosti (OWASP preporuka za Argon2id).
const (
	defaultMemoryKB = 64 * 1024
	defaultTime     = 3
	defaultThreads  = 2
	defaultSaltLen  = 16
	defaultKeyLen   = 32
)

// Granice koje cuvaju od pogresno podesenih (preslabih ili preskupih) parametara.
const (
	minSaltLen = 8
	minKeyLen  = 16
	maxMemory  = 4 * 1024 * 1024 // 4 GiB u KiB
	maxTime    = 100
	maxSalt    = 1024
	maxKey     = 1024
)

var (
	// ErrInvalidHash se vraca kada sacuvani string nije ispravan Argon2id zapis.
	ErrInvalidHash = errors.New("password: neispravan format hesa")
	// ErrIncompatibleVersion se vraca kada je heš napravljen drugom verzijom Argon2.
	ErrIncompatibleVersion = errors.New("password: nekompatibilna verzija argon2")
)

// Params su parametri Argon2id algoritma.
type Params struct {
	Memory  uint32 // KiB
	Time    uint32 // broj prolaza
	Threads uint8  // paralelizam
	SaltLen uint32 // bajtova
	KeyLen  uint32 // bajtova
}

// Validate proverava da su parametri u razumnim granicama.
func (p Params) Validate() error {
	switch {
	case p.Threads < 1:
		return errors.New("password: threads mora biti >= 1")
	case p.Time < 1 || p.Time > maxTime:
		return fmt.Errorf("password: time mora biti izmedju 1 i %d", maxTime)
	case p.Memory < 8*uint32(p.Threads) || p.Memory > maxMemory:
		return fmt.Errorf("password: memory mora biti izmedju %d i %d KiB", 8*uint32(p.Threads), maxMemory)
	case p.SaltLen < minSaltLen || p.SaltLen > maxSalt:
		return fmt.Errorf("password: salt len mora biti izmedju %d i %d", minSaltLen, maxSalt)
	case p.KeyLen < minKeyLen || p.KeyLen > maxKey:
		return fmt.Errorf("password: key len mora biti izmedju %d i %d", minKeyLen, maxKey)
	}
	return nil
}

// ParamsFromEnv cita parametre iz env promenljivih. Nepostavljene promenljive
// dobijaju podrazumevanu vrednost; postavljena a neispravna vrednost je greska.
//
//	ARGON2_MEMORY_KB, ARGON2_TIME, ARGON2_THREADS, ARGON2_SALT_LEN, ARGON2_KEY_LEN
func ParamsFromEnv() (Params, error) {
	var p Params
	var err error

	if p.Memory, err = envUint32("ARGON2_MEMORY_KB", defaultMemoryKB); err != nil {
		return Params{}, err
	}
	if p.Time, err = envUint32("ARGON2_TIME", defaultTime); err != nil {
		return Params{}, err
	}
	threads, err := envUint32("ARGON2_THREADS", defaultThreads)
	if err != nil {
		return Params{}, err
	}
	if threads > 255 {
		return Params{}, errors.New("password: ARGON2_THREADS mora biti <= 255")
	}
	p.Threads = uint8(threads)
	if p.SaltLen, err = envUint32("ARGON2_SALT_LEN", defaultSaltLen); err != nil {
		return Params{}, err
	}
	if p.KeyLen, err = envUint32("ARGON2_KEY_LEN", defaultKeyLen); err != nil {
		return Params{}, err
	}

	if err := p.Validate(); err != nil {
		return Params{}, err
	}
	return p, nil
}

func envUint32(name string, def uint32) (uint32, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("password: %s nije ispravan broj", name)
	}
	return uint32(v), nil
}

// Hash heshira lozinku sa novom nasumicnom solju i vraca PHC string.
func Hash(plain string, p Params) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}

	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: generisanje soli: %w", err)
	}

	key := argon2.IDKey([]byte(plain), salt, p.Time, p.Memory, p.Threads, p.KeyLen)

	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Time, p.Threads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify proverava lozinku u odnosu na sacuvani heš. Pogresna lozinka daje
// (false, nil); neispravan zapis daje gresku.
func Verify(plain, encoded string) (bool, error) {
	p, salt, key, err := decode(encoded)
	if err != nil {
		return false, err
	}

	other := argon2.IDKey([]byte(plain), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return subtle.ConstantTimeCompare(key, other) == 1, nil
}

// decode razlaze PHC string na parametre, so i hash. Parametre validira, da
// pokvaren ili zlonameran zapis ne bi izazvao ogromnu alokaciju ili panic.
func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash]
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return Params{}, nil, nil, ErrIncompatibleVersion
	}

	var p Params
	var threads uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &threads); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if threads < 1 || threads > 255 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	p.Threads = uint8(threads)

	enc := base64.RawStdEncoding.Strict()
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	key, err := enc.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	p.SaltLen = uint32(len(salt))
	p.KeyLen = uint32(len(key))

	if err := p.Validate(); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	return p, salt, key, nil
}
