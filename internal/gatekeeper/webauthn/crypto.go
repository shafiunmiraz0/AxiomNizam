package webauthn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// GenerateChallenge creates a 32-byte random challenge encoded as base64url (no padding).
func GenerateChallenge() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ClientData represents the WebAuthn clientDataJSON structure.
type ClientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

// ParseClientDataJSON parses and returns the client data from raw JSON.
func ParseClientDataJSON(raw []byte) (*ClientData, error) {
	var cd ClientData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return nil, fmt.Errorf("parse clientDataJSON: %w", err)
	}
	return &cd, nil
}

// HashClientDataJSON returns SHA-256 hash of the raw clientDataJSON.
func HashClientDataJSON(raw []byte) [32]byte {
	return sha256.Sum256(raw)
}

// AuthenticatorDataFlags represents the flags byte in authenticator data.
type AuthenticatorDataFlags struct {
	UserPresent            bool
	UserVerified           bool
	BackupEligible         bool
	BackupState            bool
	AttestedCredentialData bool
	ExtensionData          bool
}

// AuthenticatorData represents parsed authenticator data.
type AuthenticatorData struct {
	RPIDHash                 []byte
	Flags                    AuthenticatorDataFlags
	SignCount                uint32
	AttestedCredentialData   *AttestedCredentialData
}

// AttestedCredentialData contains credential information from registration.
type AttestedCredentialData struct {
	AAGUID              []byte
	CredentialID        []byte
	CredentialPublicKey []byte // Raw COSE key bytes
}

// ParseAuthenticatorData parses raw authenticator data bytes.
func ParseAuthenticatorData(data []byte) (*AuthenticatorData, error) {
	// Minimum: 32 (rpIdHash) + 1 (flags) + 4 (signCount) = 37 bytes
	if len(data) < 37 {
		return nil, errors.New("authenticator data too short")
	}

	ad := &AuthenticatorData{
		RPIDHash: data[:32],
	}

	flags := data[32]
	ad.Flags = AuthenticatorDataFlags{
		UserPresent:            flags&0x01 != 0,
		UserVerified:           flags&0x04 != 0,
		BackupEligible:         flags&0x08 != 0,
		BackupState:            flags&0x10 != 0,
		AttestedCredentialData: flags&0x40 != 0,
		ExtensionData:          flags&0x80 != 0,
	}

	ad.SignCount = binary.BigEndian.Uint32(data[33:37])

	offset := 37

	if ad.Flags.AttestedCredentialData {
		// Need at least 16 (AAGUID) + 2 (credIdLen) = 18 more bytes
		if len(data) < offset+18 {
			return nil, errors.New("attested credential data too short for AAGUID and credential ID length")
		}

		acd := &AttestedCredentialData{
			AAGUID: data[offset : offset+16],
		}
		offset += 16

		credIDLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2

		if len(data) < offset+credIDLen {
			return nil, errors.New("attested credential data too short for credential ID")
		}
		acd.CredentialID = data[offset : offset+credIDLen]
		offset += credIDLen

		// Remaining bytes are the COSE public key
		if offset >= len(data) {
			return nil, errors.New("attested credential data missing COSE public key")
		}
		acd.CredentialPublicKey = data[offset:]
		ad.AttestedCredentialData = acd
	}

	return ad, nil
}

// ─── CBOR (minimal) ──────────────────────────────────────────────────────────

// CBOR major types
const (
	cborMajorUint     = 0
	cborMajorNegInt   = 1
	cborMajorByteStr  = 2
	cborMajorTextStr  = 3
	cborMajorArray    = 4
	cborMajorMap      = 5
	cborMajorSemantic = 6
	cborMajorSimple   = 7
)

// DecodeCBORMap decodes a minimal CBOR map into map[int64]interface{}.
// Handles: int, negative int, byte string, text string, nested maps, arrays.
func DecodeCBORMap(data []byte) (map[int64]interface{}, error) {
	if len(data) == 0 {
		return nil, errors.New("empty CBOR data")
	}
	val, _, err := decodeCBORItem(data, 0)
	if err != nil {
		return nil, err
	}
	m, ok := val.(map[int64]interface{})
	if !ok {
		return nil, fmt.Errorf("CBOR: expected map, got %T", val)
	}
	return m, nil
}

func decodeCBORItem(data []byte, offset int) (interface{}, int, error) {
	if offset >= len(data) {
		return nil, 0, errors.New("CBOR: unexpected end of data")
	}

	b := data[offset]
	major := b >> 5

	switch major {
	case cborMajorUint:
		val, newOff, err := decodeCBORUint(data, offset)
		if err != nil {
			return nil, 0, err
		}
		return int64(val), newOff, nil

	case cborMajorNegInt:
		val, newOff, err := decodeCBORUint(data, offset)
		if err != nil {
			return nil, 0, err
		}
		return -1 - int64(val), newOff, nil

	case cborMajorByteStr:
		length, newOff, err := decodeCBORLength(data, offset)
		if err != nil {
			return nil, 0, err
		}
		if newOff+int(length) > len(data) {
			return nil, 0, errors.New("CBOR: byte string extends past end")
		}
		return data[newOff : newOff+int(length)], newOff + int(length), nil

	case cborMajorTextStr:
		length, newOff, err := decodeCBORLength(data, offset)
		if err != nil {
			return nil, 0, err
		}
		if newOff+int(length) > len(data) {
			return nil, 0, errors.New("CBOR: text string extends past end")
		}
		return string(data[newOff : newOff+int(length)]), newOff + int(length), nil

	case cborMajorArray:
		length, newOff, err := decodeCBORLength(data, offset)
		if err != nil {
			return nil, 0, err
		}
		arr := make([]interface{}, 0, length)
		for i := uint64(0); i < length; i++ {
			item, nextOff, itemErr := decodeCBORItem(data, newOff)
			if itemErr != nil {
				return nil, 0, itemErr
			}
			arr = append(arr, item)
			newOff = nextOff
		}
		return arr, newOff, nil

	case cborMajorMap:
		length, newOff, err := decodeCBORLength(data, offset)
		if err != nil {
			return nil, 0, err
		}
		m := make(map[int64]interface{}, length)
		for i := uint64(0); i < length; i++ {
			key, nextOff, keyErr := decodeCBORItem(data, newOff)
			if keyErr != nil {
				return nil, 0, keyErr
			}
			newOff = nextOff

			var intKey int64
			switch k := key.(type) {
			case int64:
				intKey = k
			case uint64:
				intKey = int64(k)
			default:
				return nil, 0, fmt.Errorf("CBOR: unsupported map key type %T", key)
			}

			val, nextOff2, valErr := decodeCBORItem(data, newOff)
			if valErr != nil {
				return nil, 0, valErr
			}
			m[intKey] = val
			newOff = nextOff2
		}
		return m, newOff, nil

	case cborMajorSimple:
		additional := data[offset] & 0x1f
		if additional == 20 {
			return false, offset + 1, nil
		}
		if additional == 21 {
			return true, offset + 1, nil
		}
		if additional == 22 {
			return nil, offset + 1, nil
		}
		return nil, 0, fmt.Errorf("CBOR: unsupported simple value %d", additional)

	default:
		return nil, 0, fmt.Errorf("CBOR: unsupported major type %d", major)
	}
}

func decodeCBORUint(data []byte, offset int) (uint64, int, error) {
	b := data[offset]
	additional := b & 0x1f

	if additional < 24 {
		return uint64(additional), offset + 1, nil
	}
	switch additional {
	case 24:
		if offset+1 >= len(data) {
			return 0, 0, errors.New("CBOR: truncated uint8")
		}
		return uint64(data[offset+1]), offset + 2, nil
	case 25:
		if offset+2 >= len(data) {
			return 0, 0, errors.New("CBOR: truncated uint16")
		}
		return uint64(binary.BigEndian.Uint16(data[offset+1 : offset+3])), offset + 3, nil
	case 26:
		if offset+4 >= len(data) {
			return 0, 0, errors.New("CBOR: truncated uint32")
		}
		return uint64(binary.BigEndian.Uint32(data[offset+1 : offset+5])), offset + 5, nil
	case 27:
		if offset+8 >= len(data) {
			return 0, 0, errors.New("CBOR: truncated uint64")
		}
		return binary.BigEndian.Uint64(data[offset+1 : offset+9]), offset + 9, nil
	default:
		return 0, 0, fmt.Errorf("CBOR: unsupported additional info %d", additional)
	}
}

func decodeCBORLength(data []byte, offset int) (uint64, int, error) {
	return decodeCBORUint(data, offset)
}

// ─── COSE Key Parsing ────────────────────────────────────────────────────────

// COSE key parameter IDs
const (
	coseKeyType   = 1
	coseKeyAlg    = 3
	coseKeyCurve  = -1
	coseKeyX      = -2
	coseKeyY      = -3
	coseKeyNegOne = -1

	coseKtyEC2    = 2
	coseAlgES256  = -7
	coseCurveP256 = 1
)

// ParseCOSEPublicKey extracts an ECDSA P-256 public key from raw COSE key bytes.
func ParseCOSEPublicKey(data []byte) (*ecdsa.PublicKey, error) {
	m, err := DecodeCBORMap(data)
	if err != nil {
		return nil, fmt.Errorf("parse COSE key: %w", err)
	}

	// Verify key type
	kty, ok := m[coseKeyType]
	if !ok {
		return nil, errors.New("COSE key missing key type")
	}
	if kty.(int64) != coseKtyEC2 {
		return nil, fmt.Errorf("unsupported COSE key type: %d", kty)
	}

	// Verify algorithm
	alg, ok := m[coseKeyAlg]
	if !ok {
		return nil, errors.New("COSE key missing algorithm")
	}
	if alg.(int64) != coseAlgES256 {
		return nil, fmt.Errorf("unsupported COSE algorithm: %d", alg)
	}

	// Verify curve
	curve, ok := m[coseKeyCurve]
	if !ok {
		return nil, errors.New("COSE key missing curve")
	}
	if curve.(int64) != coseCurveP256 {
		return nil, fmt.Errorf("unsupported COSE curve: %d", curve)
	}

	// Extract X coordinate
	xRaw, ok := m[coseKeyX]
	if !ok {
		return nil, errors.New("COSE key missing X coordinate")
	}
	xBytes, ok := xRaw.([]byte)
	if !ok {
		return nil, errors.New("COSE key X is not byte string")
	}
	if len(xBytes) != 32 {
		return nil, fmt.Errorf("COSE key X coordinate wrong length: %d", len(xBytes))
	}

	// Extract Y coordinate
	yRaw, ok := m[coseKeyY]
	if !ok {
		return nil, errors.New("COSE key missing Y coordinate")
	}
	yBytes, ok := yRaw.([]byte)
	if !ok {
		return nil, errors.New("COSE key Y is not byte string")
	}
	if len(yBytes) != 32 {
		return nil, fmt.Errorf("COSE key Y coordinate wrong length: %d", len(yBytes))
	}

	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}, nil
}

// ─── Signature Verification ──────────────────────────────────────────────────

// VerifyECDSAP256Signature verifies an ECDSA P-256 SHA-256 signature.
// sigDER is the raw DER-encoded signature (as used in WebAuthn attestation/assertion).
func VerifyECDSAP256Signature(pubKey *ecdsa.PublicKey, data []byte, sigDER []byte) bool {
	// Parse DER-encoded ECDSA-Sig-Value: SEQUENCE { r INTEGER, s INTEGER }
	r, s, err := parseDERSignature(sigDER)
	if err != nil {
		return false
	}

	hash := sha256.Sum256(data)
	return ecdsa.Verify(pubKey, hash[:], r, s)
}

// parseDERSignature parses a DER-encoded ECDSA signature into r, s components.
func parseDERSignature(der []byte) (*big.Int, *big.Int, error) {
	if len(der) < 6 {
		return nil, nil, errors.New("DER signature too short")
	}
	if der[0] != 0x30 {
		return nil, nil, errors.New("DER: expected SEQUENCE tag")
	}

	seqLen := int(der[1])
	if seqLen == 0x81 {
		if len(der) < 3 {
			return nil, nil, errors.New("DER: truncated length")
		}
		seqLen = int(der[2])
		der = der[2:]
	}

	// Parse r INTEGER
	if len(der) < 2 || der[0] != 0x02 {
		return nil, nil, errors.New("DER: expected INTEGER tag for r")
	}
	rLen := int(der[1])
	if 2+rLen > len(der) {
		return nil, nil, errors.New("DER: r length exceeds data")
	}
	r := new(big.Int).SetBytes(der[2 : 2+rLen])

	// Parse s INTEGER
	rest := der[2+rLen:]
	if len(rest) < 2 || rest[0] != 0x02 {
		return nil, nil, errors.New("DER: expected INTEGER tag for s")
	}
	sLen := int(rest[1])
	if 2+sLen > len(rest) {
		return nil, nil, errors.New("DER: s length exceeds data")
	}
	s := new(big.Int).SetBytes(rest[2 : 2+sLen])

	return r, s, nil
}

// ─── CBOR Encoder (minimal, for building verification data) ──────────────────

// EncodeCBORMap encodes a map[int]interface{} into CBOR bytes.
// Supports int keys and values that are int, int64, []byte, string.
func EncodeCBORMap(m map[int]interface{}) ([]byte, error) {
	var buf []byte
	// For simplicity, encode map header with the actual length
	buf = appendCBORHeader(buf, cborMajorMap, uint64(len(m)))

	for k, v := range m {
		buf = appendCBORIntHeader(buf, int64(k))
		var err error
		buf, err = appendCBORValue(buf, v)
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// appendCBORHeader appends a CBOR header (major type + length/additional).
func appendCBORHeader(buf []byte, major byte, val uint64) []byte {
	if val < 24 {
		return append(buf, (major<<5)|byte(val))
	}
	if val <= 0xff {
		return append(buf, (major<<5)|24, byte(val))
	}
	if val <= 0xffff {
		b := make([]byte, 3)
		b[0] = (major << 5) | 25
		binary.BigEndian.PutUint16(b[1:], uint16(val))
		return append(buf, b...)
	}
	if val <= 0xffffffff {
		b := make([]byte, 5)
		b[0] = (major << 5) | 26
		binary.BigEndian.PutUint32(b[1:], uint32(val))
		return append(buf, b...)
	}
	b := make([]byte, 9)
	b[0] = (major << 5) | 27
	binary.BigEndian.PutUint64(b[1:], val)
	return append(buf, b...)
}

// appendCBORIntHeader appends a CBOR integer header (positive or negative).
func appendCBORIntHeader(buf []byte, val int64) []byte {
	if val >= 0 {
		return appendCBORHeader(buf, cborMajorUint, uint64(val))
	}
	return appendCBORHeader(buf, cborMajorNegInt, uint64(-1-val))
}

func appendCBORValue(buf []byte, val interface{}) ([]byte, error) {
	switch v := val.(type) {
	case int64:
		return appendCBORIntHeader(buf, v), nil
	case uint64:
		return appendCBORHeader(buf, cborMajorUint, v), nil
	case int:
		return appendCBORIntHeader(buf, int64(v)), nil
	case []byte:
		buf = appendCBORHeader(buf, cborMajorByteStr, uint64(len(v)))
		buf = append(buf, v...)
		return buf, nil
	case string:
		buf = appendCBORHeader(buf, cborMajorTextStr, uint64(len(v)))
		buf = append(buf, []byte(v)...)
		return buf, nil
	default:
		return nil, fmt.Errorf("CBOR encode: unsupported type %T", val)
	}
}
