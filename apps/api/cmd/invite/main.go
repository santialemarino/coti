// Command invite mints a single-use link that lets one corralón register while public sign-up is
// closed (SIGNUP_ENABLED=false), and prints it. The link expires after SIGNUP_INVITE_TTL_DAYS.
//
//	go run ./cmd/invite [--email <address>] [--note <text>]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func main() {
	if err := run(); err != nil {
		slog.Error("invite not created", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "", "who the invite is for, kept for the team's reference")
	note := flag.String("note", "", "a note kept beside the invite, such as the corralón's name")
	flag.Parse()

	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	invites := services.NewSignupInviteService(db, repository.NewSignupInviteRepository(),
		cfg.Signup, cfg.Web, nil)
	link, invite, err := invites.Create(ctx, domain.NewSignupInvite{
		Email: optional(*email),
		Note:  optional(*note),
	})
	if err != nil {
		return err
	}

	// The link is the only copy of the token, so it goes to stdout alone and nothing logs it.
	fmt.Println(link)
	fmt.Fprintf(os.Stderr, "invite %s expires %s\n", invite.ID, invite.ExpiresAt.Format("2006-01-02 15:04 MST"))
	return nil
}

func optional(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
