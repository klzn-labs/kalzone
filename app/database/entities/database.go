package entities

import "time"

type Database struct {
	ID                 string
	Name               string
	Environment        string
	TeamID             string
	Schema             string
	Username           string
	PasswordCiphertext []byte
	PasswordNonce      []byte
	PasswordKeyID      string
	MaxConnections     int
	Adopted            bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Attachment struct {
	DatabaseID  string
	ServiceID   string
	Environment string
	AttachedAt  time.Time
	DetachedAt  *time.Time
}

func (a Attachment) Active() bool {
	return a.DetachedAt == nil
}
