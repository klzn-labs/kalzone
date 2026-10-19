package databases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"strconv"

	"github.com/klzn-labs/kalzone/app/database/entities"
)

const passwordBytes = 32

func (u DatabasesUsecase) Access(ctx context.Context, serviceID, environment string) (map[string]string, map[string]string, error) {
	db, found, err := u.store.ActiveFor(ctx, serviceID, environment)
	if err != nil || !found {
		return map[string]string{}, map[string]string{}, err
	}
	password, err := u.password(db)
	if err != nil {
		return nil, nil, err
	}
	return u.accessEnv(db), u.accessSecrets(db, password), nil
}

func (u DatabasesUsecase) accessEnv(db entities.Database) map[string]string {
	return map[string]string{"PLATFORM_DB_HOST": u.endpoint.Host, "PLATFORM_DB_PORT": strconv.Itoa(u.endpoint.Port), "PLATFORM_DB_NAME": db.Schema}
}

func (u DatabasesUsecase) accessSecrets(db entities.Database, password string) map[string]string {
	address := net.JoinHostPort(u.endpoint.Host, strconv.Itoa(u.endpoint.Port))
	return map[string]string{
		"PLATFORM_DB_USER":     db.Username,
		"PLATFORM_DB_PASSWORD": password,
		"PLATFORM_DB_DSN":      db.Username + ":" + password + "@tcp(" + address + ")/" + db.Schema + "?parseTime=true",
	}
}

func (u DatabasesUsecase) password(db entities.Database) (string, error) {
	plain, err := u.sealer.Open(db.PasswordCiphertext, db.PasswordNonce, passwordAAD(db), db.PasswordKeyID)
	return string(plain), err
}

func (u DatabasesUsecase) sealedPassword(db *entities.Database) (string, error) {
	password, err := newPassword()
	if err != nil {
		return "", err
	}
	db.PasswordCiphertext, db.PasswordNonce, db.PasswordKeyID, err = u.sealer.Seal([]byte(password), passwordAAD(*db))
	return password, err
}

func newPassword() (string, error) {
	raw := make([]byte, passwordBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func passwordAAD(db entities.Database) []byte {
	return []byte("database|" + db.ID + "|" + db.Username + "|password")
}
