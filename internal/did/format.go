package did

import (
	"errors"
	"regexp"
)

var didPattern = regexp.MustCompile(`^did:safespace:[a-zA-Z0-9_-]+$`)

func BuildDID(id string) string {
	return "did:safespace:" + id
}

func Validate(value string) error {
	if len(value) == 0 || value[:14] != "did:safespace:" {
		return errors.New("invalid did prefix")
	}
	if !didPattern.MatchString(value) {
		return errors.New("invalid did format")
	}
	return nil
}
