// Package dbproxy proxies database queries on the gateway side, enforcing
// allowlists and row limits before any SQL hits the customer DB.
package dbproxy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgProxy executes read-only SQL against a customer Postgres DB with limits.
type PgProxy struct {
	Pool         *pgxpool.Pool
	MaxRows      int
	QueryTimeout time.Duration
	Allowlist    *Allowlist
}

// NewPgProxy opens a pool against url. Caller closes the pool on shutdown.
func NewPgProxy(ctx context.Context, url string, maxConns int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		cfg.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// QueryResult is the wire shape the gateway returns.
type QueryResult struct {
	Columns         []string `json:"columns"`
	Rows            [][]any  `json:"rows"`
	RowCount        int      `json:"row_count"`
	ExecutionTimeMs int      `json:"execution_time_ms"`
}

// Execute runs a single SELECT and returns columns + rows. Rejects writes
// and queries whose tables/columns fall outside the allowlist.
func (p *PgProxy) Execute(ctx context.Context, sql string) (*QueryResult, error) {
	if p.Pool == nil {
		return nil, fmt.Errorf("no database configured on this gateway")
	}
	if p.Allowlist != nil {
		if err := p.Allowlist.Check(sql); err != nil {
			return nil, err
		}
	}

	timeout := p.QueryTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	qctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	rows, err := p.Pool.Query(qctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	descs := rows.FieldDescriptions()
	cols := make([]string, len(descs))
	for i, fd := range descs {
		cols[i] = string(fd.Name)
	}

	maxRows := p.MaxRows
	if maxRows <= 0 {
		maxRows = 10000
	}

	out := &QueryResult{Columns: cols, Rows: make([][]any, 0, 64)}
	for rows.Next() {
		if len(out.Rows) >= maxRows {
			return nil, fmt.Errorf("query returned more than max_rows_per_query=%d", maxRows)
		}
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		out.Rows = append(out.Rows, vals)
	}
	if err := rows.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("query timeout after %s", timeout)
		}
		return nil, err
	}
	out.RowCount = len(out.Rows)
	out.ExecutionTimeMs = int(time.Since(start).Milliseconds())
	return out, nil
}

// ColumnSchema describes one column of an allowlisted table.
type ColumnSchema struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// TableSchema is the column list for one allowlisted table (FQN "schema.table").
type TableSchema struct {
	Name    string         `json:"name"`
	Columns []ColumnSchema `json:"columns"`
}

type fqnParts struct{ schema, table string }

// splitFQNs splits "schema.table" entries; a bare name defaults to schema "public".
func splitFQNs(fqns []string) []fqnParts {
	out := make([]fqnParts, 0, len(fqns))
	for _, f := range fqns {
		if i := strings.IndexByte(f, '.'); i >= 0 {
			out = append(out, fqnParts{f[:i], f[i+1:]})
		} else {
			out = append(out, fqnParts{"public", f})
		}
	}
	return out
}

// filterBlockedColumns returns cols with any entry blocked by the allowlist
// removed. It uses isColumnBlocked so filtering here and query-path validation
// in walkColumns always agree. table is the unqualified table name.
func filterBlockedColumns(al *Allowlist, table string, cols []ColumnSchema) []ColumnSchema {
	if al == nil || len(al.BlockedColumns) == 0 {
		return cols
	}
	out := cols[:0:0] // nil-safe empty slice, same backing capacity
	for _, c := range cols {
		if !isColumnBlocked(al.BlockedColumns, table, c.Name) {
			out = append(out, c)
		}
	}
	return out
}

// Schema returns the columns of each allowlisted table, read from
// information_schema. Read-only; only the allowlisted tables are introspected.
// Blocked columns (per the gateway allowlist) are excluded so that the
// schema_cache only ever contains columns that query validation will accept.
func (p *PgProxy) Schema(ctx context.Context, allowedTables []string) ([]TableSchema, error) {
	if p.Pool == nil {
		return nil, fmt.Errorf("no database configured on this gateway")
	}
	parts := splitFQNs(allowedTables)
	out := make([]TableSchema, 0, len(parts))
	const q = `SELECT column_name, data_type FROM information_schema.columns
	           WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`
	for _, fp := range parts {
		rows, err := p.Pool.Query(ctx, q, fp.schema, fp.table)
		if err != nil {
			return nil, fmt.Errorf("introspect %s.%s: %w", fp.schema, fp.table, err)
		}
		ts := TableSchema{Name: fp.schema + "." + fp.table}
		for rows.Next() {
			var c ColumnSchema
			if err := rows.Scan(&c.Name, &c.Type); err != nil {
				rows.Close()
				return nil, fmt.Errorf("introspect %s.%s: %w", fp.schema, fp.table, err)
			}
			ts.Columns = append(ts.Columns, c)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("introspect %s.%s: %w", fp.schema, fp.table, err)
		}
		rows.Close()
		ts.Columns = filterBlockedColumns(p.Allowlist, fp.table, ts.Columns)
		out = append(out, ts)
	}
	return out, nil
}
