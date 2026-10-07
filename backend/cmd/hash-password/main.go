// Command hash-password prints a bcrypt hash for the given password using the
// same parameters as internal/auth.HashPassword (bcrypt.DefaultCost).
//
// Usage:
//
//	go run ./cmd/hash-password -- 'plain-password'
//	echo -n 'plain-password' | go run ./cmd/hash-password
//
// Intended for seeding a local email/password admin. Never
// log the plaintext password; only the hash is written to stdout.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/auth"
)

func main() {
	password, err := readPassword(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash-password: %v\n", err)
		os.Exit(2)
	}
	if password == "" {
		fmt.Fprintln(os.Stderr, "hash-password: empty password")
		os.Exit(2)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash-password: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(hash)
}

func readPassword(args []string) (string, error) {
	// Prefer explicit argv after "--" or first non-flag arg.
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			if i+1 >= len(args) {
				return "", fmt.Errorf("missing password after --")
			}
			return args[i+1], nil
		}
		if strings.HasPrefix(args[i], "-") {
			continue
		}
		return args[i], nil
	}
	// Fall back to stdin (no trailing newline required).
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if (info.Mode() & os.ModeCharDevice) != 0 {
		return "", fmt.Errorf("password required as arg or stdin")
	}
	data, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}
