package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// ClientMethod describes one `<OpID>WithResponse` method exposed by the
// generated client. We treat the client's AST as the source of truth for
// what `gen-platform-cmds` is allowed to call — the OpenAPI spec is the
// menu, but oapi-codegen may skip operations (multipart bodies, unsupported
// schemas) and the method signature is the contract.
type ClientMethod struct {
	OpID      string      // e.g. "DevicesPushModelToDevice" (no WithResponse suffix)
	PathArgs  []PathParam // ordered positional args before *Params
	HasParams bool        // method takes &client.<OpID>Params
	HasBody   bool        // method takes a body (typed JSONRequestBody, not WithBody/io.Reader)
	RespIdent string      // *<OpID>Response (used only for documentation)
}

// LoadClientMethods scans the generated client file and returns a map keyed
// by OpID (the method name minus "WithResponse"). Methods receiver must be
// *ClientWithResponses; ones with a *WithBody variant are skipped (we wire
// the typed-body variant instead).
func LoadClientMethods(clientFile string) (map[string]ClientMethod, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, clientFile, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", clientFile, err)
	}
	out := map[string]ClientMethod{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		if recvName(fn.Recv.List[0].Type) != "ClientWithResponses" {
			continue
		}
		if !strings.HasSuffix(fn.Name.Name, "WithResponse") {
			continue
		}
		if strings.HasSuffix(fn.Name.Name, "WithBodyWithResponse") {
			continue // io.Reader-body variant; the typed twin already covered
		}
		opID := strings.TrimSuffix(fn.Name.Name, "WithResponse")
		m := ClientMethod{OpID: opID}
		// Skip ctx (first param). Walk remaining params in order, dropping
		// the trailing variadic reqEditors.
		params := fn.Type.Params.List
		for i := 1; i < len(params); i++ {
			fld := params[i]
			ty := exprString(fld.Type)
			if strings.HasPrefix(ty, "...") {
				continue // reqEditors
			}
			switch {
			case ty == "*"+opID+"Params":
				m.HasParams = true
			case strings.HasSuffix(ty, opID+"JSONRequestBody"):
				m.HasBody = true
			default:
				// Positional path arg(s). One ast.Field may name multiple
				// args sharing a type (`a, b openapi_types.UUID`).
				goType := normalizeGoType(ty)
				count := len(fld.Names)
				if count == 0 {
					count = 1
				}
				for k := 0; k < count; k++ {
					name := ""
					if k < len(fld.Names) {
						name = fld.Names[k].Name
					}
					m.PathArgs = append(m.PathArgs, PathParam{Name: name, GoType: goType})
				}
			}
		}
		if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
			m.RespIdent = exprString(fn.Type.Results.List[0].Type)
		}
		out[opID] = m
	}
	return out, nil
}

func recvName(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		if id, ok := star.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return "*" + exprString(x.X)
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	case *ast.Ellipsis:
		return "..." + exprString(x.Elt)
	case *ast.ArrayType:
		return "[]" + exprString(x.Elt)
	}
	return ""
}

// normalizeGoType translates the client's qualifier-form for path-arg types
// into the form the generator templates emit (matching what's imported in
// `*.gen.go`). `openapi_types.UUID` is the typed alias for `uuid.UUID` —
// we accept either inbound and emit `uuid.UUID`.
func normalizeGoType(t string) string {
	switch t {
	case "openapi_types.UUID", "uuid.UUID":
		return "uuid.UUID"
	case "string":
		return "string"
	case "int", "int32", "int64":
		return "int"
	}
	return t
}
