package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

const (
	passwordScheme = "pbkdf2_sha256"
	iterations     = 120000
	saltBytes      = 16
	keyBytes       = 32
)

func HashPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", fmt.Errorf("empty password")
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := pbkdf2SHA256([]byte(password), salt, iterations, keyBytes)
	return strings.Join([]string{
		passwordScheme,
		strconv.Itoa(iterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	}, "$"), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordScheme {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) == 0 {
		return false
	}
	actual := pbkdf2SHA256([]byte(strings.TrimSpace(password)), salt, iter, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func NewSessionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	var out []byte
	var blockIndex uint32 = 1
	for len(out) < keyLen {
		u := prf(password, salt, blockIndex)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			u = prf(password, u, 0)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
		blockIndex++
	}
	return out[:keyLen]
}

func prf(password, data []byte, blockIndex uint32) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(data)
	if blockIndex > 0 {
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], blockIndex)
		mac.Write(buf[:])
	}
	return mac.Sum(nil)
}
