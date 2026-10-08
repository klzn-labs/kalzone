package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMalformed = errors.New("signature header is malformed")
	ErrExpired   = errors.New("signature is too old")
	ErrMismatch  = errors.New("signature does not match")
)

func Sign(key string, t time.Time, body []byte) string {
	stamp := strconv.FormatInt(t.Unix(), 10)
	return "t=" + stamp + ",v1=" + mac(key, stamp, body)
}

func Verify(header string, keys []string, body []byte, now time.Time, maxAge time.Duration) error {
	stamp, sent, err := parse(header)
	if err != nil {
		return err
	}
	signedAt, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return ErrMalformed
	}
	if now.Sub(time.Unix(signedAt, 0)).Abs() > maxAge {
		return ErrExpired
	}
	for _, key := range keys {
		if key != "" && hmac.Equal([]byte(mac(key, stamp, body)), []byte(sent)) {
			return nil
		}
	}
	return ErrMismatch
}

func parse(header string) (string, string, error) {
	stampPart, macPart, ok := strings.Cut(header, ",")
	stamp, okStamp := strings.CutPrefix(stampPart, "t=")
	sent, okMac := strings.CutPrefix(macPart, "v1=")
	if !ok || !okStamp || !okMac {
		return "", "", ErrMalformed
	}
	return stamp, sent, nil
}

func mac(key, stamp string, body []byte) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(stamp + "."))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
