package dbproxy

import (
	"testing"
)

func TestSplitFQNs(t *testing.T) {
	got := splitFQNs([]string{"public.orders", "sales.line_items", "bare"})
	want := []struct{ schema, table string }{
		{"public", "orders"}, {"sales", "line_items"}, {"public", "bare"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].schema != w.schema || got[i].table != w.table {
			t.Fatalf("[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestIsColumnBlocked(t *testing.T) {
	blocked := []string{"users.email", "users.ssn", "ORDERS.Internal_Note"}

	cases := []struct {
		table, col string
		want       bool
	}{
		{"users", "email", true},          // exact match
		{"users", "ssn", true},            // exact match
		{"users", "name", false},          // unblocked column
		{"orders", "internal_note", true}, // case-insensitive match
		{"ORDERS", "INTERNAL_NOTE", true}, // full upper-case
		{"orders", "id", false},           // unblocked column
		{"products", "email", false},      // same column name, different table
	}
	for _, tc := range cases {
		got := isColumnBlocked(blocked, tc.table, tc.col)
		if got != tc.want {
			t.Errorf("isColumnBlocked(%q, %q) = %v, want %v", tc.table, tc.col, got, tc.want)
		}
	}
}

func TestIsColumnBlockedEmpty(t *testing.T) {
	if isColumnBlocked(nil, "users", "email") {
		t.Fatal("nil block list should never block")
	}
	if isColumnBlocked([]string{}, "users", "email") {
		t.Fatal("empty block list should never block")
	}
}

func TestFilterBlockedColumns(t *testing.T) {
	al := &Allowlist{
		Tables:         []string{"public.users"},
		BlockedColumns: []string{"users.email", "users.ssn"},
	}
	input := []ColumnSchema{
		{Name: "id", Type: "integer"},
		{Name: "email", Type: "text"},
		{Name: "ssn", Type: "text"},
		{Name: "name", Type: "text"},
	}
	got := filterBlockedColumns(al, "users", input)
	if len(got) != 2 {
		t.Fatalf("expected 2 columns, got %d: %v", len(got), got)
	}
	if got[0].Name != "id" || got[1].Name != "name" {
		t.Fatalf("unexpected columns: %v", got)
	}
}

func TestFilterBlockedColumnsNilAllowlist(t *testing.T) {
	input := []ColumnSchema{{Name: "id"}, {Name: "email"}}
	got := filterBlockedColumns(nil, "users", input)
	if len(got) != 2 {
		t.Fatalf("nil allowlist should return all columns, got %d", len(got))
	}
}

func TestFilterBlockedColumnsNoBlockList(t *testing.T) {
	al := &Allowlist{Tables: []string{"public.users"}}
	input := []ColumnSchema{{Name: "id"}, {Name: "email"}}
	got := filterBlockedColumns(al, "users", input)
	if len(got) != 2 {
		t.Fatalf("empty block list should return all columns, got %d", len(got))
	}
}
