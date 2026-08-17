package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestConnectEmptyString(t *testing.T) {
	_, err := Connect("")
	if !errors.Is(err, ErrEmptyConnectionString) {
		t.Errorf("expected ErrEmptyConnectionString, got %v", err)
	}

	_, err = Connect("   ")
	if !errors.Is(err, ErrEmptyConnectionString) {
		t.Errorf("expected ErrEmptyConnectionString for whitespace, got %v", err)
	}
}

func TestPingNilDB(t *testing.T) {
	ctx := context.Background()
	err := Ping(ctx, nil, 5*time.Second)
	if !errors.Is(err, ErrNilDB) {
		t.Errorf("expected ErrNilDB when database handle is nil, got %v", err)
	}
}

func TestFormatError(t *testing.T) {
	timeout := 3 * time.Second

	t.Run("context deadline exceeded timeout", func(t *testing.T) {
		formatted := FormatError(context.DeadlineExceeded, timeout)
		if formatted == nil || !strings.Contains(formatted.Error(), "timed out after 3s") {
			t.Errorf("unexpected timeout error message: %v", formatted)
		}
	})

	t.Run("connection refused error", func(t *testing.T) {
		rawErr := errors.New("dial tcp 127.0.0.1:5432: connect: connection refused")
		formatted := FormatError(rawErr, timeout)
		if formatted == nil || !strings.Contains(formatted.Error(), "database unreachable: connection refused by host") {
			t.Errorf("unexpected error message: %v", formatted)
		}
	})

	t.Run("host lookup error", func(t *testing.T) {
		rawErr := errors.New("dial tcp: lookup non-existent-db-host: no such host")
		formatted := FormatError(rawErr, timeout)
		if formatted == nil || !strings.Contains(formatted.Error(), "database unreachable: host name resolution failed") {
			t.Errorf("unexpected error message: %v", formatted)
		}
	})

	t.Run("authentication error", func(t *testing.T) {
		rawErr := errors.New("password authentication failed for user \"postgres\"")
		formatted := FormatError(rawErr, timeout)
		if formatted == nil || !strings.Contains(formatted.Error(), "database authentication failed: invalid credentials") {
			t.Errorf("unexpected error message: %v", formatted)
		}
	})
}

func TestConnectAndPingUnreachable(t *testing.T) {
	ctx := context.Background()
	// Point to a non-existent port on localhost to ensure quick connection failure / timeout
	invalidDSN := "postgres://invalid:invalid@127.0.0.1:59999/testdb?sslmode=disable"
	timeout := 100 * time.Millisecond

	_, err := ConnectAndPing(ctx, invalidDSN, timeout)
	if err == nil {
		t.Fatalf("expected error connecting to unreachable database, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "unreachable") && !strings.Contains(errMsg, "timed out") && !strings.Contains(errMsg, "connection refused") {
		t.Errorf("expected actionable error message containing unreachable/timed out, got: %s", errMsg)
	}
}
