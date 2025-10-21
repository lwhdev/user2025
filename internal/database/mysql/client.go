package mysql

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/user2025/internal/config"
	"example.com/user2025/internal/database"
	"example.com/user2025/internal/models"
)

type Client struct {
	command  string
	password string
	database string
	baseArgs []string
}

func New(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}

	command := os.Getenv("MYSQL_CLIENT_PATH")
	if command == "" {
		command = "mysql"
	}

	if !filepath.IsAbs(command) {
		if _, err := exec.LookPath(command); err != nil {
			return nil, fmt.Errorf("mysql client not found: %w", err)
		}
	}

	baseArgs := []string{
		fmt.Sprintf("--host=%s", cfg.DBHost),
		fmt.Sprintf("--port=%d", cfg.DBPort),
		fmt.Sprintf("--user=%s", cfg.DBUser),
		"--batch",
		"--raw",
		"--skip-column-names",
		"--silent",
	}

	return &Client{
		command:  command,
		password: cfg.DBPassword,
		database: cfg.DBName,
		baseArgs: baseArgs,
	}, nil
}

func (c *Client) Close() error {
        return nil
}

func (c *Client) EnsureSchema(ctx context.Context) error {
	createDB := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", c.database)
	if _, err := c.run(ctx, false, createDB); err != nil {
		return fmt.Errorf("create database: %w", err)
	}

	createTable := `
                CREATE TABLE IF NOT EXISTS users (
                        id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
                        email VARCHAR(255) NOT NULL UNIQUE,
                        full_name VARCHAR(255) NOT NULL,
                        password_hash VARCHAR(255) NOT NULL,
                        created_at DATETIME NOT NULL
                ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
        `
	if _, err := c.run(ctx, true, createTable); err != nil {
		return fmt.Errorf("create users table: %w", err)
	}

	return nil
}

func (c *Client) CreateUser(ctx context.Context, user *models.User) error {
	if user == nil {
		return errors.New("user cannot be nil")
	}

	user.CreatedAt = time.Now().UTC().Truncate(time.Second)
	insert := "INSERT INTO users (email, full_name, password_hash, created_at) VALUES (?, ?, ?, ?)"
	if _, err := c.run(ctx, true, insert, user.Email, user.FullName, user.PasswordHash, user.CreatedAt); err != nil {
		if errors.Is(err, database.ErrDuplicate) {
			return err
		}
		return fmt.Errorf("insert user: %w", err)
	}

	rows, err := c.run(ctx, true, "SELECT LAST_INSERT_ID()")
	if err != nil {
		return fmt.Errorf("fetch insert id: %w", err)
	}
	if len(rows) == 0 {
		return errors.New("missing insert id")
	}

	id, err := strconv.ParseInt(strings.TrimSpace(rows[len(rows)-1]), 10, 64)
	if err != nil {
		return fmt.Errorf("parse insert id: %w", err)
	}
	user.ID = id
	return nil
}

func (c *Client) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	const query = "SELECT id, email, full_name, password_hash, created_at FROM users WHERE email = ? LIMIT 1"
	rows, err := c.run(ctx, true, query, email)
	if err != nil {
		return nil, fmt.Errorf("query user by email: %w", err)
	}
	if len(rows) == 0 || rows[0] == "" {
		return nil, database.ErrNotFound
	}
	return parseUser(rows[0])
}

func (c *Client) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	const query = "SELECT id, email, full_name, password_hash, created_at FROM users WHERE id = ? LIMIT 1"
	rows, err := c.run(ctx, true, query, id)
	if err != nil {
		return nil, fmt.Errorf("query user by id: %w", err)
	}
	if len(rows) == 0 || rows[0] == "" {
		return nil, database.ErrNotFound
	}
	return parseUser(rows[0])
}

func (c *Client) run(ctx context.Context, withDatabase bool, query string, args ...any) ([]string, error) {
	rendered, err := formatQuery(query, args...)
	if err != nil {
		return nil, err
	}

	argsList := make([]string, 0, len(c.baseArgs)+4)
	argsList = append(argsList, c.baseArgs...)
	if withDatabase {
		argsList = append(argsList, fmt.Sprintf("--database=%s", c.database))
	}
	argsList = append(argsList, "-e", rendered)

	cmd := exec.CommandContext(ctx, c.command, argsList...)
	if c.password != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", c.password))
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			msg := stderr.String()
			if msg == "" {
				msg = string(exitErr.Stderr)
			}
			msg = strings.TrimSpace(msg)
			lower := strings.ToLower(msg)
			if strings.Contains(lower, "duplicate entry") {
				return nil, database.ErrDuplicate
			}
			if strings.Contains(lower, "doesn't exist") {
				return nil, database.ErrSchemaMissing
			}
			if msg == "" {
				msg = "mysql command failed"
			}
			return nil, fmt.Errorf("mysql error: %s", msg)
		}
		return nil, err
	}

	text := strings.TrimSpace(string(output))
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, "\n"), nil
}

func parseUser(line string) (*models.User, error) {
	cols := strings.Split(line, "\t")
	if len(cols) != 5 {
		return nil, fmt.Errorf("unexpected column count: %d", len(cols))
	}

	id, err := strconv.ParseInt(cols[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse id: %w", err)
	}

	createdAt, err := time.ParseInLocation("2006-01-02 15:04:05", cols[4], time.UTC)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	return &models.User{
		ID:           id,
		Email:        cols[1],
		FullName:     cols[2],
		PasswordHash: cols[3],
		CreatedAt:    createdAt,
	}, nil
}

func formatQuery(query string, args ...any) (string, error) {
	var builder strings.Builder
	argIndex := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			if argIndex >= len(args) {
				return "", errors.New("not enough arguments for query")
			}
			builder.WriteString(encodeArg(args[argIndex]))
			argIndex++
			continue
		}
		builder.WriteByte(query[i])
	}

	if argIndex != len(args) {
		return "", errors.New("unused arguments in query")
	}

	return builder.String(), nil
}

func encodeArg(arg any) string {
	switch v := arg.(type) {
	case nil:
		return "NULL"
	case string:
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	case []byte:
		return "'" + strings.ReplaceAll(string(v), "'", "''") + "'"
	case time.Time:
		return "'" + v.UTC().Format("2006-01-02 15:04:05") + "'"
	case fmt.Stringer:
		return "'" + strings.ReplaceAll(v.String(), "'", "''") + "'"
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprintf("%v", v), "'", "''") + "'"
	}
}
