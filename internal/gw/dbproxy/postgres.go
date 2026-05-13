// Package dbproxy proxies database queries on the gateway side, enforcing
// allowlists and row limits before any SQL hits the customer DB.
package dbproxy

import (
	"context"
	"errors"
	"fmt"
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
