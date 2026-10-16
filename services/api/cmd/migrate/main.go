// Command migrate applies or rolls back database migrations.
//
// Usage:
//
//	go run ./cmd/migrate up            # apply all pending migrations
//	go run ./cmd/migrate down [N]      # roll back N (default: all) migrations
//	go run ./cmd/migrate status        # show applied and pending versions
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	args := os.Args[1:]
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate <up|down [N]|status>")
	}

	cfg, err := config.Load("migrate")
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	switch args[0] {
	case "up":
		applied, err := db.MigrateUp(ctx)
		if err != nil {
			return err
		}
		if len(applied) == 0 {
			fmt.Println("no pending migrations")
			return nil
		}
		fmt.Printf("applied migrations: %v\n", applied)
		return nil

	case "down":
		steps := 0
		if len(args) > 1 {
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 0 {
				return fmt.Errorf("invalid step count %q", args[1])
			}
			steps = n
		}
		rolledBack, err := db.MigrateDown(ctx, steps)
		if err != nil {
			return err
		}
		if len(rolledBack) == 0 {
			fmt.Println("no migrations to roll back")
			return nil
		}
		fmt.Printf("rolled back migrations: %v\n", rolledBack)
		return nil

	case "status":
		status, err := db.Status(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("current version: %d\n", status.Current)
		fmt.Printf("applied: %v\n", status.Applied)
		fmt.Printf("pending: %v\n", status.Pending)
		return nil

	default:
		return fmt.Errorf("unknown command %q (expected up, down, or status)", args[0])
	}
}
