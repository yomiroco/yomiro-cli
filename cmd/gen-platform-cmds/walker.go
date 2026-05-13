package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pb33f/libopenapi"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

type Operation struct {
	Tag         string
	OperationID string
	Method      string
	Path        string
	Summary     string
	PathParams  []string
	QueryParams []Param
	BodyType    string
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
				PathParams:  pathParamNames(op),
				QueryParams: queryParams(op),
				BodyType:    bodyType(op),
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

func pathParamNames(op *v3.Operation) []string {
	var out []string
	for _, p := range op.Parameters {
		if p.In == "path" {
			out = append(out, p.Name)
		}
	}
	return out
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
