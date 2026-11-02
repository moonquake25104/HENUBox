// genpassword prints a bcrypt hash for a password read from stdin or argv.
// Usage: go run ./cmd/genpassword [password]
package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	password := strings.Join(os.Args[1:], " ")
	if password == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/genpassword <password>")
		os.Exit(2)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash:", err)
		os.Exit(1)
	}
	fmt.Println(string(hash))
}
