package auth

import "testing"

func TestValidPhone(t *testing.T) {
	for _, phone := range []string{"+375291111111", "+447700900123"} {
		if !ValidPhone(phone) {
			t.Errorf("valid phone rejected: %s", phone)
		}
	}
	for _, phone := range []string{"375291111111", "+0001234567", "+123", "abc"} {
		if ValidPhone(phone) {
			t.Errorf("invalid phone accepted: %s", phone)
		}
	}
}

func TestPasswordHash(t *testing.T) {
	encoded, err := hashPassword("ExamplePassword123!")
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword("ExamplePassword123!", encoded) {
		t.Fatal("correct password rejected")
	}
	if checkPassword("different password", encoded) {
		t.Fatal("invalid password accepted")
	}
}
