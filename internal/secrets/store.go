package secrets

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
)

type Store struct {
	identity *age.X25519Identity
	secrets  map[string][][]byte // serviceID -> encrypted values
}

func NewStore(privateKey string) (*Store, error) {
	ids, err := age.ParseIdentities(strings.NewReader(privateKey))
	if err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no identity parsed")
	}
	xid, ok := ids[0].(*age.X25519Identity)
	if !ok {
		return nil, fmt.Errorf("identity is not X25519")
	}
	return &Store{identity: xid, secrets: make(map[string][][]byte)}, nil
}

func (s *Store) Encrypt(plaintext []byte) ([]byte, error) {
	recipient := s.identity.Recipient()
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("age encrypt: %w", err)
	}
	if _, err := io.Copy(w, bytes.NewReader(plaintext)); err != nil {
		w.Close()
		return nil, fmt.Errorf("write plaintext: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close encrypt: %w", err)
	}
	return buf.Bytes(), nil
}

func (s *Store) Decrypt(ciphertext []byte) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(ciphertext), s.identity)
	if err != nil {
		return nil, fmt.Errorf("age decrypt: %w", err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read plaintext: %w", err)
	}
	return plain, nil
}

type Redactor interface {
	Redact(s string) string
}

type replacer struct {
	r *strings.Replacer
}

func NewRedactor(secrets ...string) Redactor {
	pairs := make([]string, 0, len(secrets)*2)
	for _, s := range secrets {
		pairs = append(pairs, s, "[REDACTED]")
	}
	return &replacer{r: strings.NewReplacer(pairs...)}
}

func (r *replacer) Redact(s string) string {
	return r.r.Replace(s)
}

func (s *Store) RegisterSecret(serviceID string, plaintext []byte) error {
	enc, err := s.Encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("register secret: %w", err)
	}
	s.secrets[serviceID] = append(s.secrets[serviceID], enc)
	return nil
}

func (s *Store) RedactorFor(serviceID string) (Redactor, error) {
	encrypted, ok := s.secrets[serviceID]
	if !ok {
		return NewRedactor(), nil
	}
	secrets := make([]string, 0, len(encrypted))
	for _, enc := range encrypted {
		plain, err := s.Decrypt(enc)
		if err != nil {
			return nil, fmt.Errorf("redactor for %s: %w", serviceID, err)
		}
		secrets = append(secrets, string(plain))
	}
	return NewRedactor(secrets...), nil
}
