package model

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/leftmike/gait/config"
)

type ToolFunc func(ctx context.Context, buf []byte) (string, error)

type Tool struct {
	Name        string
	Description string
	Func        ToolFunc
	Schema      ToolSchema
}

type ToolSchema map[string]any

func structToSchema(typ reflect.Type) (map[string]any, error) {
	var req []any
	props := map[string]any{}
	for i := 0; i < typ.NumField(); i += 1 {
		var name, desc string
		var optional bool

		fld := typ.Field(i)
		jt, ok := fld.Tag.Lookup("json")
		if ok {
			vals := strings.Split(jt, ",")
			name = vals[0]
			for j := 1; j < len(vals); j += 1 {
				if vals[j] == "omitempty" || vals[j] == "omitzero" {
					optional = true
				}
			}
		}
		gt, ok := fld.Tag.Lookup("gait")
		if ok {
			if vals := strings.Split(gt, ","); len(vals) > 1 {
				return nil, fmt.Errorf("only one value allowed for gait tag: %s: %s",
					fld.Name, gt)
			}
			desc = gt
		}

		ftyp := fld.Type
		if ftyp.Kind() == reflect.Pointer {
			ftyp = ftyp.Elem()
		}

		if name == "-" {
			continue
		} else if name == "" {
			name = fld.Name
		}

		fscm, err := typeToSchema(ftyp)
		if err != nil {
			return nil, err
		}
		if desc != "" {
			fscm["description"] = desc
		}
		props[name] = fscm

		if !optional {
			req = append(req, name)
		}
	}

	scm := map[string]any{
		"properties": props,
		"type":       "object",
	}
	if len(req) > 0 {
		sort.Slice(req,
			func(i, j int) bool {
				return req[i].(string) < req[j].(string)
			})
		scm["required"] = req
	}
	return scm, nil
}

var (
	kindToSimpleJSONType = map[reflect.Kind]string{
		reflect.Bool:    "boolean",
		reflect.Int:     "integer",
		reflect.Int8:    "integer",
		reflect.Int16:   "integer",
		reflect.Int32:   "integer",
		reflect.Int64:   "integer",
		reflect.Uint:    "integer",
		reflect.Uint8:   "integer",
		reflect.Uint16:  "integer",
		reflect.Uint32:  "integer",
		reflect.Uint64:  "integer",
		reflect.Float32: "number",
		reflect.Float64: "number",
		reflect.String:  "string",
	}
)

func typeToSchema(typ reflect.Type) (map[string]any, error) {
	kind := typ.Kind()
	switch kind {
	case reflect.Struct:
		return structToSchema(typ)

	case reflect.Slice:
		items, err := typeToSchema(typ.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":  "array",
			"items": items,
		}, nil

	case reflect.Array:
		items, err := typeToSchema(typ.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":     "array",
			"maxItems": float64(typ.Len()),
			"minItems": float64(typ.Len()),
			"items":    items,
		}, nil

	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("maps must be string-keyed: %s", typ)
		}
		vals, err := typeToSchema(typ.Elem())
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"type":                 "object",
			"additionalProperties": vals,
		}, nil

	default:
		jsonType, ok := kindToSimpleJSONType[kind]
		if !ok {
			return nil, fmt.Errorf("type not supported: %s", typ)
		}
		return map[string]any{
			"type": jsonType,
		}, nil
	}
}

func NewToolSchema[T any]() (ToolSchema, error) {
	typ := reflect.TypeFor[T]()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("tool arguments must be a (pointer to a) struct: %s", typ)
	}

	return structToSchema(typ)
}

func MustToolSchema[T any]() ToolSchema {
	ts, err := NewToolSchema[T]()
	if err != nil {
		var v T
		panic(fmt.Sprintf("new tool schema failed: %T: %s", v, err))
	}
	return ts
}

func callTool(ctx context.Context, tools map[string]Tool, name string, args []byte,
	opts *config.Options) (string, error) {

	for _, tl := range tools {
		if tl.Name == name {
			return tl.Func(ctx, args)
		}
	}

	return "", fmt.Errorf("function not found: %s", name)
}
