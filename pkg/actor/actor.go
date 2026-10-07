package actor

type Actor struct {
	UserID string
	Login  string
	Admin  bool
}

func (a Actor) SignedIn() bool {
	return a.UserID != ""
}
