package entities

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/klzn-labs/kalzone/pkg/apperror"
	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
	minPassword  = 12
	maxPassword  = 128
)

var loginPattern = regexp.MustCompile(`^[a-z0-9-]{1,39}$`)

var DummyHash string

func init() {
	random, err := NewToken()
	if err != nil {
		panic(err)
	}
	if DummyHash, err = HashPassword(random); err != nil {
		panic(err)
	}
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return encodeHash(argonMemory, argonTime, argonThreads, salt, derive(password, salt, argonTime, argonMemory, argonThreads, argonKeyLen)), nil
}

func VerifyPassword(encoded, password string) bool {
	memory, passes, threads, salt, key, ok := decodeHash(encoded)
	if !ok {
		return false
	}
	derived := derive(password, salt, passes, memory, threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(derived, key) == 1
}

func CheckPassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < minPassword || length > maxPassword {
		return apperror.Invalid("use 12 to 128 characters").WithField("password")
	}
	return nil
}

func NormalizeLogin(login string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(login))
	if !loginPattern.MatchString(normalized) {
		return "", apperror.Invalid("use 1 to 39 lowercase letters, digits or dashes").WithField("username")
	}
	return normalized, nil
}

func derive(password string, salt []byte, passes, memory uint32, threads uint8, keyLen uint32) []byte {
	return argon2.IDKey([]byte(password), salt, passes, memory, threads, keyLen)
}

func encodeHash(memory, passes uint32, threads uint8, salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memory, passes, threads, b64.EncodeToString(salt), b64.EncodeToString(key))
}

func decodeHash(encoded string) (memory, passes uint32, threads uint8, salt, key []byte, ok bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return 0, 0, 0, nil, nil, false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil {
		return 0, 0, 0, nil, nil, false
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[4])
	key, keyErr := base64.RawStdEncoding.DecodeString(parts[5])
	ok = saltErr == nil && keyErr == nil && len(key) > 0 && threads > 0 && passes > 0
	return memory, passes, threads, salt, key, ok
}
