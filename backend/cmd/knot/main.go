// Command knot runs the forum API server.
//
// Usage:
//
//	knot [serve]        start the HTTP server (default)
//	knot migrate        apply database migrations and exit
//	knot passwd <user>  set a password from KNOT_NEW_PASSWORD
//	knot role <user> <role>
//	knot version
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sysvo/knot/internal/api"
	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/config"
	"github.com/sysvo/knot/internal/db"
	"github.com/sysvo/knot/internal/model"
	"github.com/sysvo/knot/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "knot:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}

	if cmd == "version" {
		fmt.Println("knot", api.Version)
		return nil
	}

	logger := newLogger()
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := db.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := db.Migrate(conn); err != nil {
		return err
	}
	st := store.New(conn)

	switch cmd {
	case "migrate":
		logger.Info("migrations applied", "db", cfg.DatabasePath)
		return nil

	case "passwd":
		if len(args) < 2 {
			return errors.New("usage: knot passwd <username>")
		}
		return setPassword(context.Background(), st, args[1])

	case "role":
		if len(args) < 3 {
			return errors.New("usage: knot role <username> <member|moderator|admin>")
		}
		return setRole(context.Background(), st, args[1], args[2])

	case "serve":
		srv := api.New(cfg, st, logger)
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := srv.Bootstrap(ctx); err != nil {
			return fmt.Errorf("bootstrap admin: %w", err)
		}
		cfg.LogSummary(logger)
		logger.Info("listening", "addr", cfg.Addr)

		if err := srv.Run(ctx); err != nil {
			return err
		}
		logger.Info("shut down cleanly")
		return nil

	default:
		return fmt.Errorf("unknown command %q (try: serve, migrate, passwd, role, version)", cmd)
	}
}

// newLogger returns a structured logger configured from the environment.
func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("KNOT_LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(os.Getenv("KNOT_LOG_JSON"), "true") {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// setPassword is the recovery path for a self-hoster who is locked out.
func setPassword(ctx context.Context, st *store.Store, username string) error {
	password := os.Getenv("KNOT_NEW_PASSWORD")
	if password == "" {
		fmt.Fprint(os.Stderr, "new password: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return err
		}
		password = strings.TrimSpace(line)
	}
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}
	acct, err := st.AccountByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user %q: %w", username, err)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if err := st.SetPassword(ctx, acct.User.ID, hash); err != nil {
		return err
	}
	if err := st.DeleteOtherSessions(ctx, acct.User.ID, ""); err != nil {
		return err
	}
	fmt.Println("password updated for", acct.User.Username)
	return nil
}

// setRole changes an account's role from the command line.
func setRole(ctx context.Context, st *store.Store, username, role string) error {
	switch role {
	case model.RoleMember, model.RoleModerator, model.RoleAdmin:
	default:
		return fmt.Errorf("unknown role %q", role)
	}
	acct, err := st.AccountByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user %q: %w", username, err)
	}
	if err := st.UpdateAccount(ctx, acct.User.ID, acct.Email, role, acct.User.Status); err != nil {
		return err
	}
	fmt.Println(acct.User.Username, "is now", role)
	return nil
}
