package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("password should verify")
	}
	if VerifyPassword("wrong password", hash) {
		t.Fatal("wrong password should not verify")
	}
}
