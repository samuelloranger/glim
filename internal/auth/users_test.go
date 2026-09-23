package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const goodPass = "correct horse battery"

func TestNormalizeUsername(t *testing.T) {
	ok := map[string]string{"sam": "sam", "  Sam ": "sam", "a.b_c-d": "a.b_c-d", strings.Repeat("a", 32): strings.Repeat("a", 32)}
	for in, want := range ok {
		got, err := NormalizeUsername(in)
		if err != nil || got != want {
			t.Errorf("NormalizeUsername(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", " ", "a b", "é", "a/b", strings.Repeat("a", 33)} {
		if _, err := NormalizeUsername(bad); !errors.Is(err, ErrInvalidUsername) {
			t.Errorf("NormalizeUsername(%q) err = %v, want ErrInvalidUsername", bad, err)
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
	u, err := db.CreateUser(ctx, "Sam", goodPass)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "sam" || u.ID == 0 {
		t.Fatalf("user = %+v", u)
	}
	if _, err := db.CreateUser(ctx, "SAM", goodPass); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	if _, err := db.Authenticate(ctx, "sam", goodPass); err != nil {
		t.Fatalf("auth good: %v", err)
	}
	if _, err := db.Authenticate(ctx, "sam", "wrong password!!"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("auth wrong pass = %v", err)
	}
	if _, err := db.Authenticate(ctx, "nobody", goodPass); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("auth unknown = %v", err)
	}
	if n, _ := db.CountUsers(ctx); n != 1 {
		t.Fatalf("count = %d", n)
	}
	if err := db.DeleteUser(ctx, "sam"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, "sam"); !errors.Is(err, ErrNoSuchUser) {
		t.Fatalf("delete twice = %v", err)
	}
}

func TestSetPassword(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	if _, err := db.CreateUser(ctx, "sam", goodPass); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPassword(ctx, "sam", "a brand new passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Authenticate(ctx, "sam", goodPass); !errors.Is(err, ErrBadCredentials) {
		t.Fatal("old password still works")
	}
	if _, err := db.Authenticate(ctx, "sam", "a brand new passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPassword(ctx, "ghost", goodPass); !errors.Is(err, ErrNoSuchUser) {
		t.Fatalf("ghost = %v", err)
	}
}

func TestListUsersSorted(t *testing.T) {
	db, _ := openTest(t)
	ctx := context.Background()
	for _, n := range []string{"zed", "amy"} {
		if _, err := db.CreateUser(ctx, n, goodPass); err != nil {
			t.Fatal(err)
		}
	}
	list, err := db.ListUsers(ctx)
	if err != nil || len(list) != 2 || list[0].Username != "amy" || list[1].Username != "zed" {
		t.Fatalf("list = %+v, %v", list, err)
	}
}
