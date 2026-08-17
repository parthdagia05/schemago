/* Migration: 0002_add_email_index.sql */
/* Description: Create index on users email column */

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
