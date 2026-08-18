package lock

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestGenerateLockID(t *testing.T) {
	id1 := GenerateLockID("schemago_migrations")
	id2 := GenerateLockID("schemago_migrations")
	id3 := GenerateLockID("custom_migrations")

	if id1 == 0 {
		t.Errorf("expected non-zero lock ID")
	}

	if id1 != id2 {
		t.Errorf("expected deterministic lock IDs for identical table names, got %d vs %d", id1, id2)
	}

	if id1 == id3 {
		t.Errorf("expected different lock IDs for different table names, got %d for both", id1)
	}
}

func TestAdvisoryLock_NilExecer(t *testing.T) {
	l := New(nil, 12345)

	err := l.Lock(context.Background())
	if !errors.Is(err, ErrNilExecer) {
		t.Fatalf("expected ErrNilExecer on Lock, got %v", err)
	}

	_, err = l.TryLock(context.Background())
	if !errors.Is(err, ErrNilExecer) {
		t.Fatalf("expected ErrNilExecer on TryLock, got %v", err)
	}
}

func TestAdvisoryLock_AcquireAndUnlock(t *testing.T) {
	db := setupTestDB(t)
	lockID := GenerateLockID("schemago_migrations")

	l := New(db, lockID)

	if l.IsLocked() {
		t.Fatalf("expected initial state to be unlocked")
	}

	err := l.Lock(context.Background())
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}

	if !l.IsLocked() {
		t.Fatalf("expected lock state to be locked")
	}

	// Double locking should fail
	err = l.Lock(context.Background())
	if !errors.Is(err, ErrAlreadyLocked) {
		t.Fatalf("expected ErrAlreadyLocked on double Lock, got %v", err)
	}

	// Unlock
	err = l.Unlock(context.Background())
	if err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	if l.IsLocked() {
		t.Fatalf("expected state to be unlocked after Unlock")
	}

	// Double unlock should be safe
	err = l.Unlock(context.Background())
	if err != nil {
		t.Fatalf("double Unlock failed: %v", err)
	}
}

func TestAdvisoryLock_TryLock(t *testing.T) {
	db := setupTestDB(t)
	lockID := GenerateLockID("schemago_migrations_try")

	l1 := New(db, lockID)
	l2 := New(db, lockID)

	ok, err := l1.TryLock(context.Background())
	if err != nil || !ok {
		t.Fatalf("l1.TryLock failed: ok=%v, err=%v", ok, err)
	}

	// l2 TryLock should fail to acquire
	ok, err = l2.TryLock(context.Background())
	if err != nil {
		t.Fatalf("l2.TryLock unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected l2.TryLock to return false when lock is held by l1")
	}

	// Unlock l1
	if err := l1.Unlock(context.Background()); err != nil {
		t.Fatalf("l1.Unlock failed: %v", err)
	}

	// Now l2 TryLock should succeed
	ok, err = l2.TryLock(context.Background())
	if err != nil || !ok {
		t.Fatalf("l2.TryLock after l1 release failed: ok=%v, err=%v", ok, err)
	}

	if err := l2.Unlock(context.Background()); err != nil {
		t.Fatalf("l2.Unlock failed: %v", err)
	}
}

func TestAdvisoryLock_ConcurrentRunners(t *testing.T) {
	db := setupTestDB(t)
	lockID := GenerateLockID("concurrent_table")

	l1 := New(db, lockID)
	l2 := New(db, lockID)

	var order []string
	var mu sync.Mutex

	record := func(name string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, name)
	}

	// Lock with runner 1
	if err := l1.Lock(context.Background()); err != nil {
		t.Fatalf("l1 Lock failed: %v", err)
	}
	record("l1_locked")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		record("l2_waiting")
		if err := l2.Lock(context.Background()); err != nil {
			t.Errorf("l2 Lock failed: %v", err)
			return
		}
		record("l2_locked")
		_ = l2.Unlock(context.Background())
		record("l2_unlocked")
	}()

	time.Sleep(50 * time.Millisecond)
	record("l1_unlocking")
	if err := l1.Unlock(context.Background()); err != nil {
		t.Fatalf("l1 Unlock failed: %v", err)
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	expectedOrder := []string{"l1_locked", "l2_waiting", "l1_unlocking", "l2_locked", "l2_unlocked"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("unexpected order length: got %v, want %v", order, expectedOrder)
	}
	for i, v := range expectedOrder {
		if order[i] != v {
			t.Errorf("order[%d] = %s, want %s (full order: %v)", i, order[i], v, order)
		}
	}
}
