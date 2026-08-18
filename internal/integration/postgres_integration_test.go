package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/parthdagia05/schemago/internal/apply"
	"github.com/parthdagia05/schemago/internal/cli"
	"github.com/parthdagia05/schemago/internal/dryrun"
	"github.com/parthdagia05/schemago/internal/history"
	"github.com/parthdagia05/schemago/internal/migration"
)

var (
	globalPostgresDSN string
	postgresOnce      sync.Once
	postgresInitErr   error
)

// getBaseDSN retrieves an existing PostgreSQL connection string or starts a Postgres container via testcontainers.
func getBaseDSN(t *testing.T) string {
	t.Helper()
	if envDSN := os.Getenv("TEST_POSTGRES_DSN"); envDSN != "" {
		return envDSN
	}
	if envDSN := os.Getenv("DATABASE_URL"); envDSN != "" {
		return envDSN
	}

	postgresOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		pgContainer, err := postgres.Run(ctx,
			"postgres:16-alpine",
			postgres.WithDatabase("postgres"),
			postgres.WithUsername("postgres"),
			postgres.WithPassword("postgres"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(1*time.Minute),
			),
		)
		if err != nil {
			postgresInitErr = fmt.Errorf("failed to start postgres container: %w", err)
			return
		}

		dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			postgresInitErr = fmt.Errorf("failed to get container DSN: %w", err)
			return
		}

		globalPostgresDSN = dsn
	})

	if postgresInitErr != nil {
		t.Fatalf("Postgres test container initialization failed: %v", postgresInitErr)
	}

	return globalPostgresDSN
}

// replaceDBNameInDSN updates the database name component in a PostgreSQL DSN.
func replaceDBNameInDSN(dsn string, newDBName string) string {
	u, err := url.Parse(dsn)
	if err == nil && u.Scheme != "" {
		u.Path = "/" + newDBName
		return u.String()
	}
	return dsn
}

// setupPostgresTestDB provisions an isolated database on the PostgreSQL instance for a single test.
func setupPostgresTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	baseDSN := getBaseDSN(t)

	adminDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Fatalf("failed to connect to admin postgres: %v", err)
	}
	defer adminDB.Close()

	dbName := fmt.Sprintf("schemago_test_%d", time.Now().UnixNano())
	_, err = adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName))
	if err != nil {
		t.Fatalf("failed to create isolated test database %s: %v", dbName, err)
	}

	testDSN := replaceDBNameInDSN(baseDSN, dbName)

	testDB, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("failed to open test database connection: %v", err)
	}

	t.Cleanup(func() {
		_ = testDB.Close()
		adminDB2, err := sql.Open("pgx", baseDSN)
		if err == nil {
			_, _ = adminDB2.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName))
			_ = adminDB2.Close()
		}
	})

	return testDB, testDSN
}

func createTempMigration(t *testing.T, dir string, version int64, filename string, content string) *migration.MigrationFile {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write migration file %s: %v", filename, err)
	}

	checksum := migration.ComputeChecksum([]byte(content))
	return &migration.MigrationFile{
		Version:  version,
		Filename: filename,
		Path:     filePath,
		Checksum: checksum,
	}
}

func tableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()
	var exists bool
	query := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_name = $1
	)`
	err := db.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check existence of table %s: %v", tableName, err)
	}
	return exists
}

// TestPostgres_FreshApplyAndHistory verifies applying migrations to a clean PostgreSQL database.
func TestPostgres_FreshApplyAndHistory(t *testing.T) {
	db, _ := setupPostgresTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_create_users.sql", `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(50) NOT NULL UNIQUE,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);
	`)
	m2 := createTempMigration(t, tempDir, 2, "0002_create_orders.sql", `
		CREATE TABLE orders (
			id SERIAL PRIMARY KEY,
			user_id INT REFERENCES users(id),
			total NUMERIC(10, 2) NOT NULL
		);
	`)

	ctx := context.Background()

	if err := history.EnsureTable(ctx, db, history.DefaultTableName); err != nil {
		t.Fatalf("EnsureTable failed: %v", err)
	}

	res, err := apply.Apply(ctx, db, history.DefaultTableName, []*migration.MigrationFile{m1, m2})
	if err != nil {
		t.Fatalf("Apply failed on real Postgres: %v", err)
	}

	if len(res.Applied) != 2 {
		t.Fatalf("expected 2 applied migrations, got %d", len(res.Applied))
	}
	if res.Failed != nil {
		t.Fatalf("expected no failed migration, got: %v", res.Failed)
	}

	if !tableExists(t, db, "users") {
		t.Errorf("table 'users' should exist in Postgres")
	}
	if !tableExists(t, db, "orders") {
		t.Errorf("table 'orders' should exist in Postgres")
	}

	applied, err := history.GetAppliedMigrations(ctx, db, history.DefaultTableName)
	if err != nil {
		t.Fatalf("GetAppliedMigrations failed: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("expected 2 history records, got %d", len(applied))
	}
	if applied[0].Version != 1 || applied[1].Version != 2 {
		t.Errorf("unexpected migration versions in history: %d, %d", applied[0].Version, applied[1].Version)
	}
}

// TestPostgres_IdempotentReapply verifies that re-running apply on an up-to-date Postgres DB is a no-op.
func TestPostgres_IdempotentReapply(t *testing.T) {
	db, _ := setupPostgresTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_create_accounts.sql", `CREATE TABLE accounts (id SERIAL PRIMARY KEY);`)
	pending := []*migration.MigrationFile{m1}

	ctx := context.Background()
	if err := history.EnsureTable(ctx, db, history.DefaultTableName); err != nil {
		t.Fatalf("EnsureTable failed: %v", err)
	}

	// First apply
	res1, err := apply.Apply(ctx, db, history.DefaultTableName, pending)
	if err != nil || len(res1.Applied) != 1 {
		t.Fatalf("initial apply failed: %v, applied count: %d", err, len(res1.Applied))
	}

	// Compute pending for second apply
	applied, err := history.GetAppliedMigrations(ctx, db, history.DefaultTableName)
	if err != nil {
		t.Fatalf("GetAppliedMigrations failed: %v", err)
	}
	discovered, err := migration.Discover(tempDir)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	remainingPending, err := history.ComputePending(discovered, applied)
	if err != nil {
		t.Fatalf("ComputePending failed: %v", err)
	}
	if len(remainingPending) != 0 {
		t.Fatalf("expected 0 pending migrations, got %d", len(remainingPending))
	}

	// Second apply (re-apply)
	res2, err := apply.Apply(ctx, db, history.DefaultTableName, remainingPending)
	if err != nil {
		t.Fatalf("re-apply failed: %v", err)
	}
	if len(res2.Applied) != 0 || res2.TotalPending != 0 {
		t.Errorf("expected 0 applied migrations on re-apply, got %d", len(res2.Applied))
	}
}

// TestPostgres_MidMigrationFailureAndRollback verifies transactional safety and rollback on SQL failure in Postgres.
func TestPostgres_MidMigrationFailureAndRollback(t *testing.T) {
	db, _ := setupPostgresTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_valid.sql", `CREATE TABLE valid_table (id SERIAL PRIMARY KEY);`)
	m2 := createTempMigration(t, tempDir, 2, "0002_invalid.sql", `
		CREATE TABLE temp_table (id INT);
		THIS IS INVALID SQL SYNTAX THAT CAUSES POSTGRES PARSE ERROR;
	`)
	m3 := createTempMigration(t, tempDir, 3, "0003_skipped.sql", `CREATE TABLE skipped_table (id SERIAL PRIMARY KEY);`)

	ctx := context.Background()
	if err := history.EnsureTable(ctx, db, history.DefaultTableName); err != nil {
		t.Fatalf("EnsureTable failed: %v", err)
	}

	res, err := apply.Apply(ctx, db, history.DefaultTableName, []*migration.MigrationFile{m1, m2, m3})
	if err == nil {
		t.Fatalf("expected error on invalid migration, got nil")
	}

	if len(res.Applied) != 1 {
		t.Fatalf("expected 1 applied migration before failure, got %d", len(res.Applied))
	}
	if res.Failed == nil || res.Failed.File.Version != 2 {
		t.Fatalf("expected failed migration to be version 2, got %v", res.Failed)
	}

	// Verify m1 was committed
	if !tableExists(t, db, "valid_table") {
		t.Errorf("valid_table (m1) should exist")
	}

	// Verify m2 was rolled back completely (temp_table should NOT exist)
	if tableExists(t, db, "temp_table") {
		t.Errorf("temp_table (m2) should have been rolled back and not exist in DB")
	}

	// Verify m3 was NOT executed
	if tableExists(t, db, "skipped_table") {
		t.Errorf("skipped_table (m3) should not exist in DB")
	}

	// Verify history table contains ONLY m1
	records, err := history.GetAppliedMigrations(ctx, db, history.DefaultTableName)
	if err != nil {
		t.Fatalf("failed to read history records: %v", err)
	}
	if len(records) != 1 || records[0].Version != 1 {
		t.Fatalf("expected 1 history record for version 1, got %d records", len(records))
	}

	// Verify fixing m2 and re-running apply succeeds
	m2Fixed := createTempMigration(t, tempDir, 2, "0002_invalid.sql", `CREATE TABLE temp_table (id INT);`)
	resFix, errFix := apply.Apply(ctx, db, history.DefaultTableName, []*migration.MigrationFile{m2Fixed, m3})
	if errFix != nil {
		t.Fatalf("apply after fix failed: %v", errFix)
	}
	if len(resFix.Applied) != 2 {
		t.Fatalf("expected 2 applied migrations on re-run, got %d", len(resFix.Applied))
	}
	if !tableExists(t, db, "temp_table") || !tableExists(t, db, "skipped_table") {
		t.Errorf("tables from m2Fixed and m3 should exist after re-run")
	}
}

// TestPostgres_ConcurrentApplyWithAdvisoryLocks verifies that concurrent apply runners using Postgres advisory locking perform safe, non-corrupt execution.
func TestPostgres_ConcurrentApplyWithAdvisoryLocks(t *testing.T) {
	_, testDSN := setupPostgresTestDB(t)
	tempDir := t.TempDir()

	for i := 1; i <= 5; i++ {
		filename := fmt.Sprintf("%04d_table_%d.sql", i, i)
		content := fmt.Sprintf("CREATE TABLE concurrent_table_%d (id SERIAL PRIMARY KEY);", i)
		createTempMigration(t, tempDir, int64(i), filename, content)
	}

	const concurrentRunners = 5
	var wg sync.WaitGroup
	errChan := make(chan error, concurrentRunners)

	for i := 0; i < concurrentRunners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var stdout, stderr bytes.Buffer
			exitCode := cli.RunWithWriters([]string{"apply", "--database-url", testDSN, "--dir", tempDir}, &stdout, &stderr)
			if exitCode != cli.ExitSuccess {
				errChan <- fmt.Errorf("runner failed with code %d, stderr: %s", exitCode, stderr.String())
			}
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrent runner error: %v", err)
	}

	// Verify final DB state using direct connection
	verifyDB, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("failed to open verification db connection: %v", err)
	}
	defer verifyDB.Close()

	ctx := context.Background()
	records, err := history.GetAppliedMigrations(ctx, verifyDB, history.DefaultTableName)
	if err != nil {
		t.Fatalf("failed to query history records: %v", err)
	}

	if len(records) != 5 {
		t.Fatalf("expected 5 applied migration history records, got %d", len(records))
	}

	for i := 1; i <= 5; i++ {
		tableName := fmt.Sprintf("concurrent_table_%d", i)
		if !tableExists(t, verifyDB, tableName) {
			t.Errorf("expected table %s to exist in Postgres", tableName)
		}
	}
}

// TestPostgres_DryRunLeavesDBClean verifies that schemago dry-run leaves Postgres completely unchanged.
func TestPostgres_DryRunLeavesDBClean(t *testing.T) {
	db, testDSN := setupPostgresTestDB(t)
	tempDir := t.TempDir()

	m1 := createTempMigration(t, tempDir, 1, "0001_create_customers.sql", `CREATE TABLE customers (id SERIAL PRIMARY KEY, name TEXT);`)
	m2 := createTempMigration(t, tempDir, 2, "0002_create_invoices.sql", `CREATE TABLE invoices (id SERIAL PRIMARY KEY, customer_id INT REFERENCES customers(id));`)

	ctx := context.Background()

	// Perform dry-run directly via package
	res, err := dryrun.DryRun(ctx, db, history.DefaultTableName, []*migration.MigrationFile{m1, m2})
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if len(res.Applied) != 2 {
		t.Fatalf("expected 2 dry-run applied items, got %d", len(res.Applied))
	}

	// Also perform dry-run via CLI
	var stdout, stderr bytes.Buffer
	exitCode := cli.RunWithWriters([]string{"dry-run", "--database-url", testDSN, "--dir", tempDir}, &stdout, &stderr)
	if exitCode != cli.ExitSuccess {
		t.Fatalf("CLI dry-run failed with code %d, stderr: %s", exitCode, stderr.String())
	}

	// Verify database remains 100% clean: no tables created, including schemago_migrations table
	if tableExists(t, db, "schemago_migrations") {
		t.Errorf("dry-run must not create history table 'schemago_migrations' in Postgres")
	}
	if tableExists(t, db, "customers") {
		t.Errorf("dry-run must not create 'customers' table in Postgres")
	}
	if tableExists(t, db, "invoices") {
		t.Errorf("dry-run must not create 'invoices' table in Postgres")
	}
}

// TestPostgres_CLIEndToEnd tests status, plan, apply, and dry-run CLI subcommands end-to-end against real Postgres.
func TestPostgres_CLIEndToEnd(t *testing.T) {
	db, testDSN := setupPostgresTestDB(t)
	tempDir := t.TempDir()

	_ = createTempMigration(t, tempDir, 1, "0001_create_products.sql", `CREATE TABLE products (id SERIAL PRIMARY KEY, title VARCHAR(100));`)

	// 1. Status on empty DB
	var stdout, stderr bytes.Buffer
	code := cli.RunWithWriters([]string{"status", "--database-url", testDSN, "--dir", tempDir, "--json"}, &stdout, &stderr)
	if code != cli.ExitFailure { // status returns ExitFailure when unapplied migrations exist
		t.Errorf("status on pending DB expected exit code %d, got %d", cli.ExitFailure, code)
	}

	// 2. Plan on pending DB
	stdout.Reset()
	stderr.Reset()
	code = cli.RunWithWriters([]string{"plan", "--database-url", testDSN, "--dir", tempDir, "--sql"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Errorf("plan command expected exit code %d, got %d, stderr: %s", cli.ExitSuccess, code, stderr.String())
	}

	// 3. Apply via CLI
	stdout.Reset()
	stderr.Reset()
	code = cli.RunWithWriters([]string{"apply", "--database-url", testDSN, "--dir", tempDir, "--json"}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Errorf("apply command expected exit code %d, got %d, stderr: %s", cli.ExitSuccess, code, stderr.String())
	}

	if !tableExists(t, db, "products") {
		t.Errorf("products table should exist after CLI apply")
	}

	// 4. Status after apply
	stdout.Reset()
	stderr.Reset()
	code = cli.RunWithWriters([]string{"status", "--database-url", testDSN, "--dir", tempDir}, &stdout, &stderr)
	if code != cli.ExitSuccess {
		t.Errorf("status after apply expected exit code %d, got %d, stderr: %s", cli.ExitSuccess, code, stderr.String())
	}
}
