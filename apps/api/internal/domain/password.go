package domain

import (
	"fmt"
	"unicode"
)

// PasswordMaxBytes is bcrypt's own limit: it hashes the first 72 bytes and refuses anything longer.
const PasswordMaxBytes = 72

// PasswordPolicy is what a password must satisfy to be stored. Every path that sets one applies it,
// so a password an administrator types for someone else cannot be weaker than one chosen at signup.
type PasswordPolicy struct {
	MinLength int
}

// Validate reports the first rule the password fails, as ErrInvalidInput.
func (p PasswordPolicy) Validate(password string) error {
	if len([]rune(password)) < p.MinLength {
		return WithCode(CodePasswordPolicy,
			fmt.Errorf("%w: password must be at least %d characters", ErrInvalidInput, p.MinLength))
	}
	// Counted in bytes, because that is the unit bcrypt truncates at: a rune count would pass an
	// accented password whose tail the hash silently ignores, and bcrypt then fails the write.
	if len(password) > PasswordMaxBytes {
		return WithCode(CodePasswordPolicy,
			fmt.Errorf("%w: password must be at most %d bytes", ErrInvalidInput, PasswordMaxBytes))
	}

	var upper, lower, digit, symbol bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case !unicode.IsLetter(r):
			symbol = true
		}
	}
	if !upper || !lower || !digit || !symbol {
		return WithCode(CodePasswordPolicy, fmt.Errorf(
			"%w: password must include an uppercase letter, a lowercase letter, a number and a symbol",
			ErrInvalidInput))
	}
	return nil
}
