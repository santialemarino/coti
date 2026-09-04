package secrets

import (
	"errors"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, KeyLength)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

func newTestSealer(t *testing.T, key []byte) *AESGCM {
	t.Helper()
	sealer, err := NewAESGCM(key)
	if err != nil {
		t.Fatalf("NewAESGCM() = %v, want no error", err)
	}
	return sealer
}

func TestAESGCM_SealOpenRoundTrip(t *testing.T) {
	t.Parallel()
	sealer := newTestSealer(t, testKey(t))

	for _, plaintext := range []string{"", "EAAG...token", strings.Repeat("s", 4096), "ñ é 漢"} {
		sealed, err := sealer.Seal(plaintext)
		if err != nil {
			t.Fatalf("Seal(%q) = %v, want no error", plaintext, err)
		}
		if sealed == plaintext {
			t.Fatalf("Seal(%q) returned the plaintext", plaintext)
		}
		if !strings.HasPrefix(sealed, "v1.") {
			t.Errorf("Seal(%q) = %q, want a v1. envelope", plaintext, sealed)
		}
		opened, err := sealer.Open(sealed)
		if err != nil {
			t.Fatalf("Open() = %v, want no error", err)
		}
		if opened != plaintext {
			t.Errorf("Open(Seal(%q)) = %q, want %q", plaintext, opened, plaintext)
		}
	}
}

func TestAESGCM_SealUsesAFreshNonce(t *testing.T) {
	t.Parallel()
	sealer := newTestSealer(t, testKey(t))

	first, err := sealer.Seal("same-token")
	if err != nil {
		t.Fatalf("Seal() = %v, want no error", err)
	}
	second, err := sealer.Seal("same-token")
	if err != nil {
		t.Fatalf("Seal() = %v, want no error", err)
	}
	if first == second {
		t.Fatalf("Seal() twice = %q both times, want distinct envelopes", first)
	}
}

func TestAESGCM_OpenRefusesWhatItDidNotSeal(t *testing.T) {
	t.Parallel()
	sealer := newTestSealer(t, testKey(t))
	sealed, err := sealer.Seal("token")
	if err != nil {
		t.Fatalf("Seal() = %v, want no error", err)
	}
	otherKey := testKey(t)
	otherKey[0] ^= 0xFF
	otherSealer := newTestSealer(t, otherKey)

	for _, test := range []struct {
		name   string
		sealer *AESGCM
		value  string
	}{
		{name: "plaintext", sealer: sealer, value: "token"},
		{name: "no prefix", sealer: sealer, value: strings.TrimPrefix(sealed, "v1.")},
		{name: "not base64", sealer: sealer, value: "v1.not base64!!"},
		{name: "shorter than a nonce", sealer: sealer, value: "v1.AAAA"},
		{name: "tampered", sealer: sealer, value: sealed[:len(sealed)-1] + "A"},
		{name: "another key", sealer: otherSealer, value: sealed},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.sealer.Open(test.value); !errors.Is(err, ErrMalformed) {
				t.Errorf("Open(%q) = %v, want %v", test.value, err, ErrMalformed)
			}
		})
	}
}

func TestAESGCM_WithoutAKeyRefusesEveryCall(t *testing.T) {
	t.Parallel()
	sealer := newTestSealer(t, nil)

	if sealer.Enabled() {
		t.Fatal("Enabled() = true, want false without a key")
	}
	if _, err := sealer.Seal("token"); !errors.Is(err, ErrNoKey) {
		t.Errorf("Seal() = %v, want %v", err, ErrNoKey)
	}
	if _, err := sealer.Open("v1.anything"); !errors.Is(err, ErrNoKey) {
		t.Errorf("Open() = %v, want %v", err, ErrNoKey)
	}
}

func TestNewAESGCM_RefusesAKeyOfTheWrongWidth(t *testing.T) {
	t.Parallel()

	for _, length := range []int{1, KeyLength - 1, KeyLength + 1} {
		if _, err := NewAESGCM(make([]byte, length)); err == nil {
			t.Errorf("NewAESGCM(%d bytes) = nil, want an error", length)
		}
	}
	sealer := newTestSealer(t, testKey(t))
	if !sealer.Enabled() {
		t.Error("Enabled() = false, want true with a full-width key")
	}
}
