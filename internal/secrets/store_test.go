package secrets_test

import (
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/LoriKarikari/paastry/internal/secrets"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	store, err := secrets.NewStore(identity.String())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	plain := "postgres://user:s3cret@host:5432/mydb"
	enc, err := store.Encrypt([]byte(plain))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(enc) == 0 {
		t.Fatal("ciphertext is empty")
	}
	if strings.Contains(string(enc), plain) {
		t.Fatal("ciphertext contains plaintext")
	}

	dec, err := store.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(dec) != plain {
		t.Fatalf("decrypted %q, want %q", dec, plain)
	}
}

func TestRedactorFullMatch(t *testing.T) {
	r := secrets.NewRedactor("postgres://user:s3cret@host:5432/mydb", "redis://:auth@cache:6379")
	out := r.Redact("connect to postgres://user:s3cret@host:5432/mydb and redis://:auth@cache:6379")
	if strings.Contains(out, "s3cret") {
		t.Fatalf("secret leaked: %s", out)
	}
	if strings.Contains(out, "auth@cache") {
		t.Fatalf("secret leaked: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("no redaction marker: %s", out)
	}
}

func TestRedactorPartialMatch(t *testing.T) {
	r := secrets.NewRedactor("s3cret", "auth")
	out := r.Redact("postgres://user:s3cret@host:5432/db?password=s3cret")
	if strings.Contains(out, "s3cret") {
		t.Fatalf("secret leaked: %s", out)
	}
	if strings.Contains(out, "auth") && !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("secret leaked: %s", out)
	}
}

func TestRedactorNoSecrets(t *testing.T) {
	r := secrets.NewRedactor()
	out := r.Redact("nothing sensitive here")
	if out != "nothing sensitive here" {
		t.Fatalf("unexpected redaction: %s", out)
	}
}

func TestRedactorEmpty(t *testing.T) {
	r := secrets.NewRedactor("secret")
	out := r.Redact("")
	if out != "" {
		t.Fatalf("got %q, want empty", out)
	}
}

func TestRedactorFor(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	store, err := secrets.NewStore(identity.String())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.RegisterSecret("svc-1", []byte("postgres://user:s3cret@host:5432/db")); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := store.RegisterSecret("svc-1", []byte("redis://:auth-token@cache:6379")); err != nil {
		t.Fatalf("register: %v", err)
	}

	r, err := store.RedactorFor("svc-1")
	if err != nil {
		t.Fatalf("redactor for: %v", err)
	}

	out := r.Redact("connecting to postgres://user:s3cret@host:5432/db then redis://:auth-token@cache:6379")
	if strings.Contains(out, "s3cret") || strings.Contains(out, "auth-token") {
		t.Fatalf("secret leaked: %s", out)
	}
}

func TestRedactorForNoSecrets(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	store, err := secrets.NewStore(identity.String())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	r, err := store.RedactorFor("unknown-svc")
	if err != nil {
		t.Fatalf("redactor for: %v", err)
	}
	out := r.Redact("nothing to redact")
	if out != "nothing to redact" {
		t.Fatalf("got %q", out)
	}
}
