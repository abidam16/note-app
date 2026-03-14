package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type stubScanner struct {
	err    error
	values []any
}

func (s stubScanner) Scan(dest ...any) error {
	if s.err != nil {
		return s.err
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			*d = s.values[i].(string)
		case *time.Time:
			*d = s.values[i].(time.Time)
		}
	}
	return nil
}

func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("expected unique violation to be detected")
	}
	if isUniqueViolation(&pgconn.PgError{Code: "23503"}) {
		t.Fatal("did not expect non-unique pg error to match")
	}
	if isUniqueViolation(errors.New("plain error")) {
		t.Fatal("did not expect plain error to match")
	}
}

func TestScanWorkspaceMember(t *testing.T) {
	now := time.Now().UTC()
	member, err := scanWorkspaceMember(stubScanner{values: []any{"m1", "w1", "u1", "owner", now, "u1", "user@example.com", "User", "secret", now, now}})
	if err != nil {
		t.Fatalf("scanWorkspaceMember success path failed: %v", err)
	}
	if member.ID != "m1" || member.User == nil || member.User.PasswordHash != "" {
		t.Fatalf("unexpected scanned member: %+v", member)
	}

	if _, err := scanWorkspaceMember(stubScanner{err: errors.New("scan failed")}); err == nil || err.Error() != "scan failed" {
		t.Fatalf("expected scan error propagation, got %v", err)
	}
}
