// Package linkseal keeps a guest link's address sealed with a key derived from the hub's own secret (HUB_SECRET_KEY,
// or the key file in the data directory), so whoever runs the access can copy the address again while only its hash
// opens the link. A hub whose key changed cannot open the older addresses: those links get new ones.
package linkseal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

const keyInfo = "harnessly-access-link/v1"

// Sealer seals the tokens of one hub.
type Sealer struct {
	id   string
	aead cipher.AEAD
}

// New derives the links' key from the hub's master key; nil without one.
func New(master []byte) *Sealer {
	if len(master) == 0 {
		return nil
	}
	key, err := hkdf.Key(sha256.New, master, []byte("harnessly"), keyInfo, 32)
	if err != nil {
		return nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(key)
	return &Sealer{id: hex.EncodeToString(sum[:4]), aead: aead}
}

// The sealed value is bound to the table and the link it belongs to: copied to another row it does not open.
func aad(table, linkID string) []byte { return []byte(keyInfo + "\x00" + table + "\x00" + linkID) }

// Seal is the token sealed for one link of a table: "<key id>.<nonce>.<sealed>"; "" without a key.
func (s *Sealer) Seal(table, linkID, token string) string {
	if s == nil {
		return ""
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return ""
	}
	out := s.aead.Seal(nil, nonce, []byte(token), aad(table, linkID))
	enc := base64.RawURLEncoding
	return s.id + "." + enc.EncodeToString(nonce) + "." + enc.EncodeToString(out)
}

// Open is the token back; false for another key, another link or a damaged value.
func (s *Sealer) Open(table, linkID, sealed string) (string, bool) {
	if s == nil || sealed == "" {
		return "", false
	}
	parts := strings.Split(sealed, ".")
	if len(parts) != 3 || parts[0] != s.id {
		return "", false
	}
	enc := base64.RawURLEncoding
	nonce, err := enc.DecodeString(parts[1])
	if err != nil || len(nonce) != s.aead.NonceSize() {
		return "", false
	}
	box, err := enc.DecodeString(parts[2])
	if err != nil {
		return "", false
	}
	plain, err := s.aead.Open(nil, nonce, box, aad(table, linkID))
	if err != nil {
		return "", false
	}
	return string(plain), true
}
