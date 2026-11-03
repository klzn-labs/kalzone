package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/cmd/controlplane/options"
	"github.com/klzn-labs/kalzone/internal/migrator"
	"go.uber.org/fx"
)

const (
	setupLinkCommand = "setup-link"
	setupLinkTimeout = 30 * time.Second
)

type linkMaker interface {
	NewSetupLink(ctx context.Context) (string, error)
	NewResetLinkFor(ctx context.Context, login string) (string, error)
}

func runSetupLink(args []string, stdout, stderr io.Writer) int {
	reset, err := parseSetupLinkArgs(args, stderr)
	if err != nil {
		return 1
	}
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = stderr
	var url string
	app := fx.New(options.Options(), fx.NopLogger, fx.Invoke(func(m *migrator.Migrator, u accounts.SetupUsecase) error {
		url, err = migratedLink(m, u, reset)
		return err
	}))
	if app.Err() != nil {
		fmt.Fprintln(stderr, rootCause(app.Err(), err))
		return 1
	}
	fmt.Fprintln(stdout, url)
	return 0
}

func parseSetupLinkArgs(args []string, stderr io.Writer) (string, error) {
	flags := flag.NewFlagSet(setupLinkCommand, flag.ContinueOnError)
	flags.SetOutput(stderr)
	reset := flags.String("reset", "", "print a password reset link for this login instead")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument %q\n", flags.Arg(0))
		return "", errors.New("unexpected argument")
	}
	return *reset, nil
}

func migratedLink(m *migrator.Migrator, u linkMaker, reset string) (string, error) {
	if err := errors.Join(m.Up(), m.Close()); err != nil {
		return "", err
	}
	return makeLink(u, reset)
}

func makeLink(u linkMaker, reset string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), setupLinkTimeout)
	defer cancel()
	if reset != "" {
		return u.NewResetLinkFor(ctx, reset)
	}
	return u.NewSetupLink(ctx)
}

func rootCause(appErr, linkErr error) error {
	if linkErr != nil {
		return linkErr
	}
	return appErr
}
