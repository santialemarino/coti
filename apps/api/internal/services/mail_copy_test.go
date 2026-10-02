package services

import (
	"strings"
	"testing"
)

// The validity line is the one piece of mail copy with branches in it, and it reads as a whole
// sentence to the recipient — "vence en 1 horas" is the failure to catch.
func TestPasswordResetValidity_ReadsAsASentence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		minutes int
		want    string
	}{
		{"under an hour", 30, "El enlace vence en 30 minutos y se puede usar una sola vez."},
		{"exactly one hour", 60, "El enlace vence en 1 hora y se puede usar una sola vez."},
		{"whole hours", 120, "El enlace vence en 2 horas y se puede usar una sola vez."},
		{"not a whole hour", 90, "El enlace vence en 90 minutos y se puede usar una sola vez."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := passwordResetValidity(tc.minutes); got != tc.want {
				t.Fatalf("passwordResetValidity(%d) = %q, want %q", tc.minutes, got, tc.want)
			}
		})
	}
}

func TestEmailVerificationValidity_ReadsAsASentence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		hours int
		want  string
	}{
		{"under a day", 12, "El enlace vence en 12 horas y se puede usar una sola vez."},
		{"exactly one day", 24, "El enlace vence en 1 día y se puede usar una sola vez."},
		{"whole days", 48, "El enlace vence en 2 días y se puede usar una sola vez."},
		{"not a whole day", 36, "El enlace vence en 36 horas y se puede usar una sola vez."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := emailVerificationValidity(tc.hours); got != tc.want {
				t.Fatalf("emailVerificationValidity(%d) = %q, want %q", tc.hours, got, tc.want)
			}
		})
	}
}

func TestInviteValidity_ReadsAsASentence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		hours int
		want  string
	}{
		{"one hour", 1, "La invitación vence en 1 hora y se puede usar una sola vez. Si se vence, pedile a un administrador que te la reenvíe."},
		{"exactly one day", 24, "La invitación vence en 1 día y se puede usar una sola vez. Si se vence, pedile a un administrador que te la reenvíe."},
		{"a week", 168, "La invitación vence en 7 días y se puede usar una sola vez. Si se vence, pedile a un administrador que te la reenvíe."},
		{"not a whole day", 36, "La invitación vence en 36 horas y se puede usar una sola vez. Si se vence, pedile a un administrador que te la reenvíe."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := inviteValidity(tc.hours); got != tc.want {
				t.Fatalf("inviteValidity(%d) = %q, want %q", tc.hours, got, tc.want)
			}
		})
	}
}

// Every link is single-use and every mail says so; a recipient who has already used one needs to
// know that is why it stopped working, not that something broke.
func TestValidity_AlwaysStatesSingleUse(t *testing.T) {
	t.Parallel()
	for _, got := range []string{
		passwordResetValidity(30),
		passwordResetValidity(60),
		emailVerificationValidity(24),
		emailVerificationValidity(36),
		inviteValidity(24),
		inviteValidity(36),
	} {
		if !strings.Contains(got, "una sola vez") {
			t.Errorf("validity copy does not state single use: %q", got)
		}
	}
}
