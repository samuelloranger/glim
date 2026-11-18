package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/config"
	"golang.org/x/term"
)

var stdin = bufio.NewReader(os.Stdin)

func cmdUser(args []string) error {
	db, err := auth.Open(config.DBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	return runUser(args, db, stdin, os.Stdout)
}

func runUser(args []string, db *auth.DB, in *bufio.Reader, out io.Writer) error {
	ctx := context.Background()
	usage := fmt.Errorf("usage: glim user ls | passwd <email> | rm <email>")
	if len(args) == 0 {
		return usage
	}
	switch args[0] {
	case "ls", "list":
		users, err := db.ListUsers(ctx)
		if err != nil {
			return err
		}
		if len(users) == 0 {
			fmt.Fprintln(out, "no accounts yet (open the dashboard to create the first one)")
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "EMAIL\tCREATED")
		for _, u := range users {
			fmt.Fprintf(w, "%s\t%s\n", u.Email, u.CreatedAt.Local().Format(time.RFC1123))
		}
		return w.Flush()
	case "passwd":
		if len(args) != 2 {
			return usage
		}
		if _, err := db.GetUser(ctx, args[1]); err != nil {
			return fmt.Errorf("no such user: %s", args[1])
		}
		p1, err := readPassword(in, "New password: ")
		if err != nil {
			return err
		}
		p2, err := readPassword(in, "Repeat password: ")
		if err != nil {
			return err
		}
		if p1 != p2 {
			return fmt.Errorf("passwords do not match")
		}
		if err := db.SetPassword(ctx, args[1], p1); err != nil {
			return passwordError(err)
		}
		fmt.Fprintf(out, "password updated for %s (signed out everywhere)\n", args[1])
		return nil
	case "rm", "remove":
		if len(args) != 2 {
			return usage
		}
		if err := db.DeleteUser(ctx, args[1]); err != nil {
			return fmt.Errorf("no such user: %s", args[1])
		}
		fmt.Fprintln(out, "removed", args[1])
		return nil
	default:
		return usage
	}
}

func passwordError(err error) error {
	switch err {
	case auth.ErrPasswordTooShort:
		return fmt.Errorf("password must be at least %d characters", auth.MinPasswordChars)
	case auth.ErrPasswordTooLong:
		return fmt.Errorf("password must be at most %d bytes", auth.MaxPasswordBytes)
	}
	return err
}

// readPassword prompts on stderr; it hides input on a terminal and reads a
// plain line otherwise (scripts, tests).
func readPassword(in *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if in == stdin && term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
