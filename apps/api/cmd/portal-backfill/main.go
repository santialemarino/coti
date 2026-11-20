// Command portal-backfill gives every account without a portal of its own (none at all, or the
// bare shared host) its default /c/<label> portal, derived from the account's name.
//
// The hostname depends on CUSTOMER_PORTAL_BASE_URL, which only the running configuration knows,
// so this is a command rather than a migration. It prints the plan and writes nothing unless
// --apply is given; running it again finds nothing left to do.
//
//	go run ./cmd/portal-backfill [--apply]
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func main() {
	if err := run(); err != nil {
		slog.Error("portal backfill failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	apply := flag.Bool("apply", false, "write the assignments instead of only printing them")
	flag.Parse()

	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Cross-account by nature: it reads and writes every account's routing name.
	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	portals := services.NewDefaultPortalService(db, repository.NewCustomerPortalRepository(), cfg.CustomerPortal)
	assignments, err := portals.Backfill(ctx, *apply)
	if err != nil {
		return err
	}
	for _, a := range assignments {
		log.Info("default portal", slog.String("account", a.AccountID.String()),
			slog.String("name", a.AccountName), slog.String("previous", a.Previous),
			slog.String("hostname", a.Hostname))
	}
	log.Info("portal backfill finished", slog.Int("accounts", len(assignments)), slog.Bool("applied", *apply))
	return nil
}
