package entities

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/adrianozp/platform/pkg/apperror"
)

type ReferenceKind string

const (
	ByID     ReferenceKind = "id"
	ByLatest ReferenceKind = "latest"
	ByNumber ReferenceKind = "number"
	ByCommit ReferenceKind = "commit"
	ByTag    ReferenceKind = "tag"
)

const ReferenceForms = "a release id, latest, r<number>, sha:<7 to 40 hex characters> or a tag"

var (
	idPattern     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	numberPattern = regexp.MustCompile(`^r[1-9][0-9]{0,8}$`)
	prefixPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	errReference  = apperror.Invalid("a release is named by " + ReferenceForms).WithHint("send latest, r42, sha:abc1234 or a tag such as 1.4.0")
	errCommit     = apperror.Invalid("a commit reference is sha: followed by 7 to 40 hexadecimal characters").WithHint("use only 0-9 and a-f, for example sha:abc1234")
)

type Reference struct {
	Kind   ReferenceKind
	Raw    string
	Number int
	Prefix string
}

func ParseReference(raw string) (Reference, error) {
	switch {
	case idPattern.MatchString(raw):
		return Reference{Kind: ByID, Raw: raw}, nil
	case raw == string(ByLatest):
		return Reference{Kind: ByLatest, Raw: raw}, nil
	case numberPattern.MatchString(raw):
		number, err := strconv.Atoi(raw[1:])
		return Reference{Kind: ByNumber, Raw: raw, Number: number}, err
	case strings.HasPrefix(raw, "sha:"):
		return commitReference(raw)
	case tagPattern.MatchString(raw):
		return Reference{Kind: ByTag, Raw: raw}, nil
	}
	return Reference{}, errReference
}

func commitReference(raw string) (Reference, error) {
	prefix := strings.ToLower(strings.TrimPrefix(raw, "sha:"))
	if !prefixPattern.MatchString(prefix) {
		return Reference{}, errCommit
	}
	return Reference{Kind: ByCommit, Raw: raw, Prefix: prefix}, nil
}
