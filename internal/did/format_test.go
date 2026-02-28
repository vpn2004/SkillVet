package did

import (
	"strings"
	"testing"
)

func TestBuildDIDUsesSafespaceMethod(t *testing.T) {
	got := BuildDID("abc123")
	want := "did:safespace:abc123"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestValidateAcceptsValidDID(t *testing.T) {
	err := Validate("did:safespace:node-01")
	if err != nil {
		t.Fatalf("expected valid DID, got error %v", err)
	}
}

func TestValidateRejectsWrongPrefix(t *testing.T) {
	err := Validate("did:example:node-01")
	if err == nil || !strings.Contains(err.Error(), "prefix") {
		t.Fatalf("expected prefix error, got %v", err)
	}
}
