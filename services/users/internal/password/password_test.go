package password

import (
	"errors"
	"strings"
	"testing"
)

// Mali parametri da testovi budu brzi.
var testParams = Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestVerify_CorrectPassword(t *testing.T) {
	h, err := Hash("macka123", testParams)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := Verify("macka123", h)
	if err != nil || !ok {
		t.Fatalf("ocekivano (true, nil), dobijeno (%v, %v)", ok, err)
	}
}

func TestVerify_WrongPassword(t *testing.T) {
	h, _ := Hash("macka123", testParams)
	ok, err := Verify("pas456", h)
	if err != nil || ok {
		t.Fatalf("ocekivano (false, nil), dobijeno (%v, %v)", ok, err)
	}
}

func TestHash_DifferentSaltEachTime(t *testing.T) {
	a, _ := Hash("macka123", testParams)
	b, _ := Hash("macka123", testParams)
	if a == b {
		t.Fatal("dva heša iste lozinke moraju se razlikovati (nasumična so)")
	}
}

func TestHash_NoPlaintext(t *testing.T) {
	h, _ := Hash("macka123", testParams)
	if strings.Contains(h, "macka123") {
		t.Fatal("heš ne sme sadržati lozinku u čistom tekstu")
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("neočekivan format: %s", h)
	}
}

func TestVerify_UsesParamsFromHash(t *testing.T) {
	other := Params{Memory: 128, Time: 2, Threads: 2, SaltLen: 12, KeyLen: 24}
	h, _ := Hash("tajna", other)
	ok, err := Verify("tajna", h)
	if err != nil || !ok {
		t.Fatalf("ocekivano (true, nil), dobijeno (%v, %v)", ok, err)
	}
}

func TestVerify_MalformedHash(t *testing.T) {
	good, _ := Hash("x", testParams)
	cases := map[string]string{
		"prazno":           "",
		"smece":            "nije heš",
		"pogresan alg":     strings.Replace(good, "argon2id", "argon2i", 1),
		"manje delova":     "$argon2id$v=19$m=64,t=1,p=1$abc",
		"los base64":       "$argon2id$v=19$m=64,t=1,p=1$!!!$!!!",
		"los parametar":    "$argon2id$v=19$m=x,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"nula threadova":   "$argon2id$v=19$m=64,t=1,p=0$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"ogromna memorija": "$argon2id$v=19$m=4294967295,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	for name, enc := range cases {
		t.Run(name, func(t *testing.T) {
			ok, err := Verify("x", enc)
			if ok || err == nil {
				t.Fatalf("ocekivana greska, dobijeno (%v, %v)", ok, err)
			}
		})
	}
}

func TestVerify_IncompatibleVersion(t *testing.T) {
	h, _ := Hash("x", testParams)
	h = strings.Replace(h, "v=19", "v=16", 1)
	if _, err := Verify("x", h); !errors.Is(err, ErrIncompatibleVersion) {
		t.Fatalf("ocekivano ErrIncompatibleVersion, dobijeno %v", err)
	}
}

func TestHash_InvalidParams(t *testing.T) {
	bad := testParams
	bad.Threads = 0
	if _, err := Hash("x", bad); err == nil {
		t.Fatal("ocekivana greska za threads=0")
	}
}

func TestParamsFromEnv_Defaults(t *testing.T) {
	for _, k := range []string{"ARGON2_MEMORY_KB", "ARGON2_TIME", "ARGON2_THREADS", "ARGON2_SALT_LEN", "ARGON2_KEY_LEN"} {
		t.Setenv(k, "")
	}
	p, err := ParamsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	want := Params{Memory: 65536, Time: 3, Threads: 2, SaltLen: 16, KeyLen: 32}
	if p != want {
		t.Fatalf("ocekivano %+v, dobijeno %+v", want, p)
	}
}

func TestParamsFromEnv_Custom(t *testing.T) {
	t.Setenv("ARGON2_MEMORY_KB", "1024")
	t.Setenv("ARGON2_TIME", "2")
	t.Setenv("ARGON2_THREADS", "4")
	p, err := ParamsFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if p.Memory != 1024 || p.Time != 2 || p.Threads != 4 {
		t.Fatalf("neočekivani parametri: %+v", p)
	}
}

func TestParamsFromEnv_Invalid(t *testing.T) {
	cases := map[string]string{
		"ARGON2_THREADS":   "0",
		"ARGON2_TIME":      "abc",
		"ARGON2_MEMORY_KB": "-5",
		"ARGON2_SALT_LEN":  "2",
	}
	for k, v := range cases {
		t.Run(k+"="+v, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := ParamsFromEnv(); err == nil {
				t.Fatal("ocekivana greska")
			}
		})
	}
}

func TestVerify_BothHashesOfSamePasswordValid(t *testing.T) {
	a, _ := Hash("macka123", testParams)
	b, _ := Hash("macka123", testParams)
	for _, h := range []string{a, b} {
		if ok, err := Verify("macka123", h); err != nil || !ok {
			t.Fatalf("ocekivano (true, nil), dobijeno (%v, %v)", ok, err)
		}
	}
}

func TestVerify_SimilarPasswordsRejected(t *testing.T) {
	h, _ := Hash("Macka123", testParams)
	for _, wrong := range []string{"macka123", "Macka124", "Macka123 ", "Macka12", ""} {
		ok, err := Verify(wrong, h)
		if err != nil || ok {
			t.Fatalf("lozinka %q: ocekivano (false, nil), dobijeno (%v, %v)", wrong, ok, err)
		}
	}
}

func TestHashVerify_UnicodeAndEmpty(t *testing.T) {
	for _, pw := range []string{"šđčćž-ŠĐČĆŽ", "лозинка", "", strings.Repeat("a", 1000)} {
		h, err := Hash(pw, testParams)
		if err != nil {
			t.Fatalf("Hash(%q): %v", pw, err)
		}
		if ok, err := Verify(pw, h); err != nil || !ok {
			t.Fatalf("Verify(%q): dobijeno (%v, %v)", pw, ok, err)
		}
	}
}
