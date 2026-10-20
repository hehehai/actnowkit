package licence

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// TokenFormat is the first part of every licence token.
const TokenFormat = "anl1"

// Editions of a licence (docs/licensing.md §1).
const (
	EditionWhitelist = "whitelist" // early access, free, until paid_since
	EditionPaid      = "paid"
	EditionManual    = "manual" // issued by an admin
)

// Token is the signed content of a licence token (docs/licensing.md §5.2).
// Unknown fields are ignored.
type Token struct {
	V          int     `json:"v"`
	CodeID     string  `json:"cid"`
	Activation string  `json:"aid"`
	Product    string  `json:"prd"`
	App        string  `json:"app"`
	Edition    string  `json:"ed"`
	Device     string  `json:"dev"`
	Version    string  `json:"ver"`
	PaidSince  *string `json:"ps"`
	IssuedAt   int64   `json:"iat"`
	RefreshAt  int64   `json:"rfa"`
	ExpiresAt  int64   `json:"exp"`
	CodeExpiry *int64  `json:"lexp"`
	MaxDevices int     `json:"md"`
	Nonce      string  `json:"nonce"`

	// KeyID is the kid the token was signed with (not part of the payload).
	KeyID string `json:"-"`
	// Raw is the token as received.
	Raw string `json:"-"`
}

// Issued, RefreshAfter, Expires, CodeExpires as times.
func (t *Token) Issued() time.Time       { return time.Unix(t.IssuedAt, 0) }
func (t *Token) RefreshAfter() time.Time { return time.Unix(t.RefreshAt, 0) }
func (t *Token) Expires() time.Time      { return time.Unix(t.ExpiresAt, 0) }

// CodeExpires is when the code itself ends; zero for perpetual codes.
func (t *Token) CodeExpires() time.Time {
	if t.CodeExpiry == nil {
		return time.Time{}
	}
	return time.Unix(*t.CodeExpiry, 0)
}

// PaidSinceVersion is the token's paid_since, "" when none.
func (t *Token) PaidSinceVersion() string {
	if t.PaidSince == nil {
		return ""
	}
	return *t.PaidSince
}

// Errors of ParseToken.
var (
	ErrTokenFormat    = errors.New("licence: not a licence token")
	ErrTokenKey       = errors.New("licence: token signed with an unknown key")
	ErrTokenSignature = errors.New("licence: token signature is invalid")
)

var kidPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// ParseToken verifies a token's Ed25519 signature with the key its kid
// names in keys, then decodes it. A token that fails is never decoded.
func ParseToken(raw string, keys map[string]ed25519.PublicKey) (*Token, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 4 || parts[0] != TokenFormat || !kidPattern.MatchString(parts[1]) {
		return nil, ErrTokenFormat
	}
	key, ok := keys[parts[1]]
	if !ok || len(key) != ed25519.PublicKeySize {
		return nil, ErrTokenKey
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, ErrTokenSignature
	}
	signed := parts[0] + "." + parts[1] + "." + parts[2]
	if !ed25519.Verify(key, []byte(signed), sig) {
		return nil, ErrTokenSignature
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrTokenFormat
	}
	var t Token
	if err := json.Unmarshal(body, &t); err != nil || t.V != 1 || t.CodeID == "" {
		return nil, ErrTokenFormat
	}
	t.KeyID = parts[1]
	t.Raw = strings.TrimSpace(raw)
	return &t, nil
}

// ParsePublicKey reads a base64 Ed25519 public key (32 bytes), as
// GET /api/licence/v1/keys gives it.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("licence: a public key is 32 bytes of base64")
	}
	return ed25519.PublicKey(raw), nil
}

// ProductionKeyID and ProductionPublicKey are actnow.dev's signing key
// (docs/licensing.md §5.3). Apps embed ProductionKeys; a key rotation adds
// one here before the site switches to it.
const (
	ProductionKeyID     = "lk-2026-10"
	ProductionPublicKey = "BUaM27MRaB2gNN9pVp6sGwy5l0YId7r6FmVnU5hQTdQ="
)

// ProductionKeys are the public keys released apps trust.
func ProductionKeys() map[string]ed25519.PublicKey {
	key, err := ParsePublicKey(ProductionPublicKey)
	if err != nil {
		panic(err)
	}
	return map[string]ed25519.PublicKey{ProductionKeyID: key}
}
