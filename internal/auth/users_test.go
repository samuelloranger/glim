package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const goodPass = "correct horse battery"

func TestNormalizeEmail(t *testing.T) {
	ok := map[string]string{
		"sam@example.com":        "sam@example.com",
		"  Sam@Example.COM ":     "sam@example.com",
		"a.b+tag@sub.example.io": "a.b+tag@sub.example.io",
	}
	for in, want := range ok {
		got, err := NormalizeEmail(in)
		if err != nil || got != want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	long := strings.Repeat("a", 250) + "@x.io" // 255 bytes
	for _, bad := range []string{"", " ", "sam", "sam@", "@example.com", "sam@example", "a b@example.com", "a@b@example.com", long} {
		if _, err := NormalizeEmail(bad); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("NormalizeEmail(%q) err = %v, want ErrInvalidEmail", bad, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("short: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("x", 73)); !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("73 bytes: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("x", 72)); err != nil {
		t.Errorf("72 bytes: %v", err)
	}
	if err := ValidatePassword(goodPass); err != nil {
		t.Errorf("good: %v", err)
	}
}

func TestCreateAuthenticateDelete(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	u, err := db.CreateUser(ctx, "Sam@Example.com", goodPass)
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "sam@example.com" || u.ID == 0 {
		t.Fatalf("user = %+v", u)
	}
	if _, err := db.CreateUser(ctx, "SAM@EXAMPLE.COM", goodPass); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	if _, err := db.Authenticate(ctx, "sam@example.com", goodPass); err != nil {
		t.Fatalf("auth good: %v", err)
	}
	if _, err := db.Authenticate(ctx, "sam@example.com", "wrong password!!"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("auth wrong pass = %v", err)
	}
	if _, err := db.Authenticate(ctx, "nobody@example.com", goodPass); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("auth unknown = %v", err)
	}
	if n, _ := db.CountUsers(ctx); n != 1 {
		t.Fatalf("count = %d", n)
	}
	if err := db.DeleteUser(ctx, "sam@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, "sam@example.com"); !errors.Is(err, ErrNoSuchUser) {
		t.Fatalf("delete twice = %v", err)
	}
}

func TestSetPassword(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	if _, err := db.CreateUser(ctx, "sam@example.com", goodPass); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPassword(ctx, "sam@example.com", "a brand new passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Authenticate(ctx, "sam@example.com", goodPass); !errors.Is(err, ErrBadCredentials) {
		t.Fatal("old password still works")
	}
	if _, err := db.Authenticate(ctx, "sam@example.com", "a brand new passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPassword(ctx, "ghost@example.com", goodPass); !errors.Is(err, ErrNoSuchUser) {
		t.Fatalf("ghost = %v", err)
	}
}

func TestListUsersSorted(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	for _, n := range []string{"zed@example.com", "amy@example.com"} {
		if _, err := db.CreateUser(ctx, n, goodPass); err != nil {
			t.Fatal(err)
		}
	}
	list, err := db.ListUsers(ctx)
	if err != nil || len(list) != 2 || list[0].Email != "amy@example.com" || list[1].Email != "zed@example.com" {
		t.Fatalf("list = %+v, %v", list, err)
	}
}
