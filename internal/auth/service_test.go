package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	encoded, err := hashPassword("correct horse battery staple")
	if err != nil { t.Fatal(err) }

	ok, err := verifyPassword("correct horse battery staple", encoded)
	if err != nil { t.Fatal(err) }
	if !ok { t.Fatal("expected password to verify") }

	ok, err = verifyPassword("wrong password", encoded)
	if err != nil { t.Fatal(err) }
	if ok { t.Fatal("expected wrong password to fail") }
}

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail("  PERSON@Example.COM "); got != "person@example.com" {
		t.Fatalf("got %q", got)
	}
}
