package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

type Operation struct {
	Tag         string
	OperationID string
	Method      string
	Path        string
	Summary     string
	PathParams  []PathParam
	QueryParams []Param
	BodyType    string
	// BodyFields lists the top-level properties of the request body schema,
	// used to render the schema-preview help block and the --skeleton JSON
	// template. Nil when there's no body or the body schema isn't an object.
	BodyFields []BodyField
}

// BodyField is one top-level property of a request body's JSON schema.
// Nested objects collapse to Kind="object"; the skeleton renderer emits
// `{}` for them and operators fill in via --json-body @file.
type BodyField struct {
	Name        string
	Kind        string // "string", "integer", "number", "boolean", "object", "array", "uuid"
	Format      string // raw OpenAPI format (uuid, date-time, ...) for richer hints
	Description string
	Required    bool
	Enum        []string
}

// PathParam carries the Go type the generated client expects for this
// positional argument. Format inferred from the OpenAPI parameter schema —
// `string + format=uuid` → "uuid.UUID", plain integer → "int", anything
// else → "string". Anything more exotic blows up obviously during build.
type PathParam struct {
	Name   string
	GoType string
}

type Param struct {
	Name     string
	GoType   string
	Required bool
}

func Walk(path string) (map[string][]Operation, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := libopenapi.NewDocument(b)
	if err != nil {
		return nil, err
	}
	model, err := doc.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("build model: %w", err)
	}

	out := map[string][]Operation{}
	if model.Model.Paths == nil {
		return out, nil
	}
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		ops := map[string]*v3.Operation{
			"GET":    item.Get,
			"POST":   item.Post,
			"PUT":    item.Put,
			"PATCH":  item.Patch,
			"DELETE": item.Delete,
		}
		for method, op := range ops {
			if op == nil {
				continue
			}
			tag := primaryTag(op)
			if tag == "" {
				continue
			}
			out[tag] = append(out[tag], Operation{
				Tag:         tag,
				OperationID: op.OperationId,
				Method:      method,
				Path:        path,
				Summary:     op.Summary,
				PathParams:  pathParams(path, op),
				QueryParams: queryParams(op),
				BodyType:    bodyType(op),
				BodyFields:  bodyFields(op),
			})
		}
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool { return out[k][i].OperationID < out[k][j].OperationID })
	}
	return out, nil
}

func primaryTag(op *v3.Operation) string {
	if len(op.Tags) == 0 {
		return ""
	}
	return op.Tags[0]
}

// pathParams returns path parameters in URL-template order — matching
// oapi-codegen's method-signature ordering — annotated with the Go type the
// generated client expects (uuid.UUID for `format: uuid`, int for integer
// types, plain string otherwise).
func pathParams(path string, op *v3.Operation) []PathParam {
	types := map[string]string{}
	for _, p := range op.Parameters {
		if p.In == "path" {
			types[p.Name] = goTypeForParam(p)
		}
	}
	var out []PathParam
	for {
		i := strings.Index(path, "{")
		if i < 0 {
			break
		}
		j := strings.Index(path[i:], "}")
		if j < 0 {
			break
		}
		name := path[i+1 : i+j]
		if gt, ok := types[name]; ok {
			out = append(out, PathParam{Name: name, GoType: gt})
		}
		path = path[i+j+1:]
	}
	return out
}

func goTypeForParam(p *v3.Parameter) string {
	if p.Schema == nil {
		return "string"
	}
	s := p.Schema.Schema()
	if s == nil || len(s.Type) == 0 {
		return "string"
	}
	switch s.Type[0] {
	case "integer":
		return "int"
	case "string":
		if s.Format == "uuid" {
			return "uuid.UUID"
		}
		return "string"
	}
	return "string"
}

func queryParams(op *v3.Operation) []Param {
	var out []Param
	for _, p := range op.Parameters {
		if p.In != "query" {
			continue
		}
		gt := "string"
		if p.Schema != nil {
			s := p.Schema.Schema()
			if s != nil && len(s.Type) > 0 {
				switch s.Type[0] {
				case "integer":
					gt = "int"
				case "boolean":
					gt = "bool"
				}
			}
		}
		out = append(out, Param{Name: p.Name, GoType: gt, Required: p.Required != nil && *p.Required})
	}
	return out
}

func bodyType(op *v3.Operation) string {
	if op.RequestBody == nil || op.RequestBody.Content == nil {
		return ""
	}
	for ct, mt := range op.RequestBody.Content.FromOldest() {
		if !strings.HasPrefix(ct, "application/json") {
			continue
		}
		if mt.Schema == nil {
			continue
		}
		s := mt.Schema.Schema()
		if mt.Schema.IsReference() {
			parts := strings.Split(mt.Schema.GetReference(), "/")
			return parts[len(parts)-1]
		}
		_ = s
	}
	return ""
}

// bodyFields resolves the request body schema and returns its top-level
// properties annotated with kind/format/required/enum metadata. Returns nil
// when the body isn't an `application/json` object schema (multipart,
// references the generator can't resolve, oneOf/anyOf top-level, etc.).
func bodyFields(op *v3.Operation) []BodyField {
	if op.RequestBody == nil || op.RequestBody.Content == nil {
		return nil
	}
	for ct, mt := range op.RequestBody.Content.FromOldest() {
		if !strings.HasPrefix(ct, "application/json") {
			continue
		}
		if mt.Schema == nil {
			continue
		}
		s := mt.Schema.Schema()
		if s == nil || s.Properties == nil {
			return nil
		}
		required := map[string]bool{}
		for _, name := range s.Required {
			required[name] = true
		}
		var out []BodyField
		for name, propProxy := range s.Properties.FromOldest() {
			prop := propProxy.Schema()
			if prop == nil {
				continue
			}
			out = append(out, BodyField{
				Name:        name,
				Kind:        schemaKind(prop),
				Format:      prop.Format,
				Description: collapseWhitespace(prop.Description),
				Required:    required[name],
				Enum:        enumStrings(prop),
			})
		}
		return out
	}
	return nil
}

func schemaKind(s *base.Schema) string {
	if s == nil || len(s.Type) == 0 {
		return "any"
	}
	t := s.Type[0]
	if t == "string" && s.Format == "uuid" {
		return "uuid"
	}
	return t
}

func enumStrings(s *base.Schema) []string {
	if s == nil || len(s.Enum) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.Enum))
	for _, e := range s.Enum {
		if e == nil {
			continue
		}
		out = append(out, e.Value)
	}
	return out
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
