package dbproxy

import (
	"strings"
	"testing"
)

func mustAllowlist(t *testing.T) *Allowlist {
	a := &Allowlist{
		Tables:         []string{"public.defects", "inspections.runs"},
		BlockedColumns: []string{"users.email", "users.ssn"},
		ReadOnly:       true,
	}
	return a
}

func TestAllowlistAcceptsAllowedTable(t *testing.T) {
	a := mustAllowlist(t)
	if err := a.Check("SELECT count(*) FROM public.defects"); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestAllowlistRejectsUnlistedTable(t *testing.T) {
	a := mustAllowlist(t)
	err := a.Check("SELECT * FROM secrets.passwords")
	if err == nil || !strings.Contains(err.Error(), "secrets.passwords") {
		t.Fatalf("expected rejection, got %v", err)
	}
}

func TestAllowlistRejectsWriteWhenReadOnly(t *testing.T) {
	a := mustAllowlist(t)
	err := a.Check("DELETE FROM public.defects WHERE id=1")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Fatalf("expected read-only rejection, got %v", err)
	}
}

func TestAllowlistRejectsBlockedColumn(t *testing.T) {
	a := mustAllowlist(t)
	a.Tables = append(a.Tables, "users")
	err := a.Check("SELECT users.email FROM users")
	if err == nil || !strings.Contains(err.Error(), "users.email") {
		t.Fatalf("expected blocked column rejection, got %v", err)
	}
}

func TestAllowlistAcceptsJoinAcrossAllowedTables(t *testing.T) {
	a := mustAllowlist(t)
	err := a.Check(`SELECT d.id, r.created_at FROM public.defects d JOIN inspections.runs r ON r.defect_id = d.id`)
	if err != nil {
		t.Fatalf("expected accepted join, got %v", err)
	}
}
