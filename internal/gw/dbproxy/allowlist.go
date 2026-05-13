package dbproxy

import (
	"fmt"
	"strings"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver" // required for parser to handle literals
)

// Allowlist filters SQL by table FQN and blocked column references.
type Allowlist struct {
	Tables         []string // FQNs: "schema.table"
	BlockedColumns []string // FQNs: "table.column"
	ReadOnly       bool
}

// Check returns nil if the SQL is admissible, or an error describing the violation.
//
// Rules:
//   - Only SELECT (and CTEs whose top-level is SELECT) are allowed when ReadOnly.
//   - Every table reference must be in Tables (matched as schema.table or table).
//   - No column reference may match a BlockedColumns entry.
func (a *Allowlist) Check(sql string) error {
	p := parser.New()
	stmts, _, err := p.Parse(sql, "", "")
	if err != nil {
		return fmt.Errorf("sql parse: %w", err)
	}
	for _, stmt := range stmts {
		if a.ReadOnly {
			if _, ok := stmt.(*ast.SelectStmt); !ok {
				if _, ok := stmt.(*ast.SetOprStmt); !ok {
					return fmt.Errorf("read-only: only SELECT statements are allowed")
				}
			}
		}
		if err := a.walkTables(stmt); err != nil {
			return err
		}
		if err := a.walkColumns(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (a *Allowlist) walkTables(node ast.Node) error {
	allowed := map[string]struct{}{}
	for _, t := range a.Tables {
		allowed[strings.ToLower(t)] = struct{}{}
	}
	var firstErr error
	visitor := tableVisitor{check: func(schema, name string) {
		if firstErr != nil {
			return
		}
		fqn := strings.ToLower(name)
		if schema != "" {
			fqn = strings.ToLower(schema) + "." + strings.ToLower(name)
		}
		if _, ok := allowed[fqn]; ok {
			return
		}
		// Also accept short names that match an allowlist entry's table portion.
		for entry := range allowed {
			if parts := strings.SplitN(entry, ".", 2); len(parts) == 2 && parts[1] == strings.ToLower(name) {
				return
			}
		}
		firstErr = fmt.Errorf("table %q is not in the gateway allowlist", fqn)
	}}
	node.Accept(&visitor)
	return firstErr
}

type tableVisitor struct {
	check func(schema, name string)
}

func (v *tableVisitor) Enter(n ast.Node) (ast.Node, bool) {
	if t, ok := n.(*ast.TableName); ok {
		v.check(t.Schema.O, t.Name.O)
	}
	return n, false
}
func (v *tableVisitor) Leave(n ast.Node) (ast.Node, bool) { return n, true }

func (a *Allowlist) walkColumns(node ast.Node) error {
	blocked := map[string]struct{}{}
	for _, c := range a.BlockedColumns {
		blocked[strings.ToLower(c)] = struct{}{}
	}
	if len(blocked) == 0 {
		return nil
	}
	var firstErr error
	visitor := columnVisitor{check: func(table, name string) {
		if firstErr != nil {
			return
		}
		if table == "" {
			return
		}
		fqn := strings.ToLower(table) + "." + strings.ToLower(name)
		if _, ok := blocked[fqn]; ok {
			firstErr = fmt.Errorf("column %q is blocked by gateway config", fqn)
		}
	}}
	node.Accept(&visitor)
	return firstErr
}

type columnVisitor struct {
	check func(table, name string)
}

func (v *columnVisitor) Enter(n ast.Node) (ast.Node, bool) {
	if cn, ok := n.(*ast.ColumnNameExpr); ok && cn.Name != nil {
		v.check(cn.Name.Table.O, cn.Name.Name.O)
	}
	return n, false
}
func (v *columnVisitor) Leave(n ast.Node) (ast.Node, bool) { return n, true }
