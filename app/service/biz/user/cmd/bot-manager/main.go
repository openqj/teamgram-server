package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const dsnEnvironmentVariable = "TEAMGRAM_OPS_MYSQL_DSN"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bot-manager:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: bot-manager grant|revoke --bot-id ID --reason REASON")
	}
	var enabled bool
	switch args[0] {
	case "grant":
		enabled = true
	case "revoke":
	case "-h", "--help", "help":
		fmt.Println("usage: bot-manager grant|revoke --bot-id ID --reason REASON")
		return nil
	default:
		return errors.New("operation must be grant or revoke")
	}

	flags := flag.NewFlagSet("bot-manager "+args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	botID := flags.Int64("bot-id", 0, "registered bot ID")
	reason := flags.String("reason", "", "required operational ticket or change reason")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if len(flags.Args()) != 0 || *botID <= 0 {
		return errors.New("provide one positive --bot-id")
	}
	cleanReason := strings.TrimSpace(*reason)
	if cleanReason == "" || !utf8.ValidString(cleanReason) || utf8.RuneCountInString(cleanReason) > 512 || strings.IndexFunc(cleanReason, unicode.IsControl) >= 0 {
		return errors.New("--reason must contain 1 to 512 non-control characters")
	}
	dsn := strings.TrimSpace(os.Getenv(dsnEnvironmentVariable))
	if dsn == "" {
		return fmt.Errorf("%s is required", dsnEnvironmentVariable)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var oldEnabled, deleted bool
	var userType int32
	err = tx.QueryRowContext(ctx, `SELECT b.bot_can_manage_bots, u.user_type, u.deleted
		FROM bots b JOIN users u ON u.id=b.bot_id
		WHERE b.bot_id=? FOR UPDATE`, *botID).Scan(&oldEnabled, &userType, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("bot %d is not registered", *botID)
	}
	if err != nil {
		return fmt.Errorf("load bot: %w", err)
	}
	if userType != user.UserTypeBot || deleted {
		return fmt.Errorf("user %d is not an active bot", *botID)
	}
	var operator string
	if err = tx.QueryRowContext(ctx, "SELECT CURRENT_USER()").Scan(&operator); err != nil {
		return fmt.Errorf("identify database operator: %w", err)
	}
	if oldEnabled != enabled {
		var result sql.Result
		if result, err = tx.ExecContext(ctx, `UPDATE bots SET bot_can_manage_bots=? WHERE bot_id=?`, enabled, *botID); err != nil {
			return fmt.Errorf("update bot capability: %w", err)
		}
		if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
			if affectedErr != nil {
				return fmt.Errorf("verify bot capability update: %w", affectedErr)
			}
			return fmt.Errorf("bot capability update affected %d rows", affected)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO bot_manager_capability_audit
			(bot_id, old_enabled, new_enabled, operator_db_user, reason)
			VALUES (?,?,?,?,?)`, *botID, oldEnabled, enabled, operator, cleanReason); err != nil {
			return fmt.Errorf("record capability change: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit capability change: %w", err)
	}
	fmt.Printf("bot_id=%d bot_can_manage_bots=%t changed=%t operator=%s\n", *botID, enabled, oldEnabled != enabled, operator)
	return nil
}
