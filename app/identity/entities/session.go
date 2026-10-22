package entities

import (
	"errors"
	"time"
)

func OpenSession(userID string, now time.Time, ttl time.Duration) (Session, IssuedSession, error) {
	token, tokenErr := NewToken()
	csrf, csrfErr := NewToken()
	if err := errors.Join(tokenErr, csrfErr); err != nil {
		return Session{}, IssuedSession{}, err
	}
	session := Session{TokenHash: HashToken(token), CSRFToken: csrf, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	return session, IssuedSession{Token: token, CSRFToken: csrf, ExpiresAt: session.ExpiresAt}, nil
}
