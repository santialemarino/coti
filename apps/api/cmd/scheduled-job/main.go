// Command scheduled-job runs one unit of scheduled work and records what it swept.
//
// The deployment platform owns the schedule: one job component per task, each with its own cron
// and its own --job argument, so the frequency is configuration rather than a value compiled in.
//
// It builds no mailer. Correction learning may call the embedding provider, but a scheduled
// process still cannot reach a client: nothing goes out without a seller deciding it should.
//
//	go run ./cmd/scheduled-job --list
//	go run ./cmd/scheduled-job --job <name>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	aiprovider "github.com/santialemarino/coti/apps/api/internal/ai/provider"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func main() {
	if err := run(); err != nil {
		slog.Error("scheduled job failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("job", "", "name of the job to run")
	list := flag.Bool("list", false, "print the registered jobs and exit")
	flag.Parse()
	// A stray argument is usually a mistyped flag, and silently ignoring it would run the wrong
	// job or none at all on a schedule nobody is watching.
	if flag.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flag.Arg(0))
	}

	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, cfg.Job.Timeout)
	defer cancel()

	// The owner pool, deliberately: a sweep crosses every account, which is the case row level
	// security is switched on but not forced for.
	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	providers, err := aiprovider.Bind(cfg.AI, log)
	if err != nil {
		return err
	}

	// Each of the four planned tasks — pending attachments, quote expiry, the follow-up sweep and
	// closing message windows — registers itself here from its own feature's ticket.
	corrections := repository.NewQuoteCorrectionRepository()
	jobs, err := services.NewJobService(db, repository.NewJobRunRepository(), log,
		services.NewQuoteCorrectionJob(corrections, providers.Embedder, cfg.QuoteCorrection))
	if err != nil {
		return err
	}

	if *list {
		fmt.Println(strings.Join(jobs.Names(), "\n"))
		return nil
	}
	if *name == "" {
		return errors.New("--job is required; --list prints the registered jobs")
	}

	if _, err := jobs.Run(ctx, *name); err != nil {
		return err
	}
	return nil
}
