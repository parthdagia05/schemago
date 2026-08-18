// Package lock implements PostgreSQL advisory locking for safe concurrent migration execution across multiple runners or deployment pods.
package lock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
)

var (
	// ErrNilExecer indicates an uninitialized database handle was passed to AdvisoryLock.
	ErrNilExecer = errors.New("database execer handle is nil")

	// ErrAlreadyLocked indicates that lock acquisition was attempted on an AdvisoryLock instance that is already locked.
	ErrAlreadyLocked = errors.New("advisory lock is already acquired")

	// globalFallbackLocks maintains in-memory locks for non-PostgreSQL drivers (e.g., SQLite during unit tests).
	globalFallbackLocks = struct {
		sync.Mutex
		locks map[int64]*sync.Mutex
	}{
		locks: make(map[int64]*sync.Mutex),
	}
)

// getFallbackMutex retrieves or creates an in-memory fallback mutex for a given lockID.
func getFallbackMutex(lockID int64) *sync.Mutex {
	globalFallbackLocks.Lock()
	defer globalFallbackLocks.Unlock()
	m, ok := globalFallbackLocks.locks[lockID]
	if !ok {
		m = &sync.Mutex{}
		globalFallbackLocks.locks[lockID] = m
	}
	return m
}

// GenerateLockID computes a deterministic 64-bit signed integer lock key from the schema history table name.
func GenerateLockID(tableName string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("schemago:" + tableName))
	return int64(h.Sum64())
}

// QueryerExecer represents a database handle capable of running row queries.
type QueryerExecer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// AdvisoryLock manages a session-level PostgreSQL advisory lock, falling back to process-level synchronization for non-Postgres drivers.
type AdvisoryLock struct {
	execer     QueryerExecer
	lockID     int64
	locked     bool
	isFallback bool
	mu         sync.Mutex
}

// New creates a new AdvisoryLock instance for the given database handle and lock ID.
func New(execer QueryerExecer, lockID int64) *AdvisoryLock {
	return &AdvisoryLock{
		execer: execer,
		lockID: lockID,
	}
}

// Lock acquires the advisory lock, blocking until it is obtained or context is canceled.
func (l *AdvisoryLock) Lock(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.locked {
		return ErrAlreadyLocked
	}

	if l.execer == nil {
		return ErrNilExecer
	}

	var dummy any
	err := l.execer.QueryRowContext(ctx, "SELECT pg_advisory_lock($1)", l.lockID).Scan(&dummy)
	if err != nil {
		if isNonPostgresError(err) {
			m := getFallbackMutex(l.lockID)
			m.Lock()
			l.isFallback = true
			l.locked = true
			return nil
		}
		return fmt.Errorf("failed to acquire postgres advisory lock (%d): %w", l.lockID, err)
	}

	l.locked = true
	return nil
}

// TryLock attempts to acquire the advisory lock without blocking.
// Returns (true, nil) if lock was acquired, (false, nil) if lock is currently held by another session.
func (l *AdvisoryLock) TryLock(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.locked {
		return true, nil
	}

	if l.execer == nil {
		return false, ErrNilExecer
	}

	var acquired bool
	err := l.execer.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", l.lockID).Scan(&acquired)
	if err != nil {
		if isNonPostgresError(err) {
			m := getFallbackMutex(l.lockID)
			if m.TryLock() {
				l.isFallback = true
				l.locked = true
				return true, nil
			}
			return false, nil
		}
		return false, fmt.Errorf("failed to try postgres advisory lock (%d): %w", l.lockID, err)
	}

	if acquired {
		l.locked = true
	}
	return acquired, nil
}

// Unlock releases the advisory lock if currently held by this AdvisoryLock instance.
func (l *AdvisoryLock) Unlock(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.locked {
		return nil
	}

	if l.isFallback {
		m := getFallbackMutex(l.lockID)
		m.Unlock()
		l.locked = false
		l.isFallback = false
		return nil
	}

	var released bool
	err := l.execer.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", l.lockID).Scan(&released)
	l.locked = false
	if err != nil {
		return fmt.Errorf("failed to release postgres advisory lock (%d): %w", l.lockID, err)
	}

	if !released {
		return fmt.Errorf("postgres advisory lock (%d) was not held by session", l.lockID)
	}

	return nil
}

// IsLocked returns whether this AdvisoryLock instance currently holds the lock.
func (l *AdvisoryLock) IsLocked() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.locked
}

// isNonPostgresError determines if an error indicates a database driver that does not support PostgreSQL advisory locks (e.g. SQLite).
func isNonPostgresError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such function") ||
		strings.Contains(msg, "unknown function") ||
		strings.Contains(msg, "sqlite") ||
		strings.Contains(msg, "near \"select\"") ||
		strings.Contains(msg, "syntax error")
}
