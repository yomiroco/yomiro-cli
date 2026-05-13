package main

import (
	"fmt"
	"strconv"
	"strings"
)

// buildLongDoc renders the cobra `Long` text for a body-bearing operation:
// the human summary, a column-aligned "Request body fields" table, and a
// short note pointing operators at --skeleton for a starter template.
//
// Field rows are indented with two spaces and aligned on three columns
// (name, kind, required marker). Enum values are appended as "one of:
// a, b, c". Descriptions wrap inline when they fit and otherwise are
// truncated — the goal is `--help` discoverability, not full reference.
func buildLongDoc(op Operation) string {
	var b strings.Builder
	if op.Summary != "" {
		b.WriteString(op.Summary)
		b.WriteString(".\n\n")
	}
	b.WriteString("Request body fields:\n")
	nameW := 4
	kindW := 4
	for _, f := range op.BodyFields {
		if len(f.Name) > nameW {
			nameW = len(f.Name)
		}
		if len(f.Kind) > kindW {
			kindW = len(f.Kind)
		}
	}
	for _, f := range op.BodyFields {
		marker := "optional"
		if f.Required {
			marker = "required"
		}
		fmt.Fprintf(&b, "  %-*s  %-*s  %s", nameW, f.Name, kindW, f.Kind, marker)
		extras := []string{}
		if len(f.Enum) > 0 {
			extras = append(extras, "one of: "+strings.Join(f.Enum, ", "))
		}
		if f.Description != "" {
			extras = append(extras, f.Description)
		}
		if len(extras) > 0 {
			b.WriteString("  ")
			b.WriteString(truncate(strings.Join(extras, "; "), 80))
		}
		b.WriteString("\n")
	}
	b.WriteString("\nRun with --skeleton to print a starter JSON template you can edit\n")
	b.WriteString("and replay via --json-body @body.json.\n")
	return b.String()
}

// buildSkeleton emits a JSON object literal with one entry per top-level
// body field. Required fields get a type-appropriate placeholder (empty
// string, zero UUID, 0, false); optional fields are emitted as `null` so
// operators can clearly distinguish "I want to send this" from "leave it
// unset" by editing or deleting lines. Nested objects/arrays render as
// `{}` / `[]` — operators write deeper structure by hand.
func buildSkeleton(fields []BodyField) string {
	if len(fields) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteString("{\n")
	for i, f := range fields {
		fmt.Fprintf(&b, "  %q: %s", f.Name, skeletonValue(f))
		if i < len(fields)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
	return b.String()
}

func skeletonValue(f BodyField) string {
	if !f.Required {
		return "null"
	}
	if len(f.Enum) > 0 {
		return strconv.Quote(f.Enum[0])
	}
	switch f.Kind {
	case "uuid":
		return `"00000000-0000-0000-0000-000000000000"`
	case "string":
		return `""`
	case "integer", "number":
		return "0"
	case "boolean":
		return "false"
	case "array":
		return "[]"
	case "object":
		return "{}"
	default:
		return "null"
	}
}

// quoteForTemplate produces a Go double-quoted string literal that's safe
// to splice directly into the generator's text/template — strconv.Quote
// handles every escape (newlines, quotes, control chars) so we never need
// to think about backtick collisions with operationId summaries that
// contain code fragments.
func quoteForTemplate(s string) string { return strconv.Quote(s) }

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
