package model

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestFieldNameToJSON(t *testing.T) {
	cases := []struct {
		nam, json string
	}{
		{"UserID", "user_id"},
		{"URLValue", "url_value"},
		{"CreatedAt", "created_at"},
		{"HTTPRequest", "http_request"},
		{"Xyz", "xyz"},
		{"xyz", "xyz"},
		{"XyZ", "xy_z"},
		{"xyZ", "xy_z"},
		{"XYZ", "xyz"},
		{"abcDef", "abc_def"},
		{"AbcDef", "abc_def"},
		{"abcDEFGhi", "abc_def_ghi"},
		{"AbcDEFGhi", "abc_def_ghi"},
	}

	for _, c := range cases {
		json := fieldNameToJSON(c.nam)
		if json != c.json {
			t.Errorf("fieldNameToJSON(%s) got %s want %s", c.nam, json, c.json)
		}
	}
}

func walkSchema(scm map[string]any, fn func(scm map[string]any)) {
	fn(scm)

	for _, val := range scm {
		if val, ok := val.(map[string]any); ok {
			walkSchema(val, fn)
		}
	}
}

func sortRequired(scm map[string]any) {
	val, ok := scm["required"]
	if ok {
		req := val.([]any)
		sort.Slice(req,
			func(i, j int) bool {
				return req[i].(string) < req[j].(string)
			})
	}
}

func jsonSchema(typ reflect.Type) (map[string]any, error) {
	sc, err := jsonschema.ForType(typ, nil)
	if err != nil {
		return nil, err
	}

	buf, err := json.Marshal(sc)
	if err != nil {
		return nil, err
	}

	var scm map[string]any
	err = json.Unmarshal(buf, &scm)
	if err != nil {
		return nil, err
	}

	walkSchema(scm,
		func(scm map[string]any) {
			sortRequired(scm)
			delete(scm, "maximum")
			delete(scm, "minimum")
			val, ok := scm["additionalProperties"]
			if ok && reflect.DeepEqual(val, false) {
				delete(scm, "additionalProperties")
			}
		})
	return scm, nil
}

func normalizeSchema(scm map[string]any) {
	walkSchema(scm,
		func(scm map[string]any) {
			sortRequired(scm)
			delete(scm, "format")
		})
}

func TestTypeToSchema(t *testing.T) {
	type struct1 struct {
		Value string `json:"value"`
	}

	cases := []any{
		"string",
		int(123),
		int8(123),
		int16(123),
		int32(123),
		int64(123),
		uint8(123),
		uint16(123),
		uint32(123),
		uint64(123),
		float32(12.3),
		float64(12.3),
		false,
		[]int{},
		[]uint32{},
		[2]string{},
		struct {
			ID    int      `json:"id"`
			Name  string   `json:"name,omitempty"`
			Value *float64 `json:"value"`
		}{},
		struct {
			ID    *int    `json:"id,omitempty"`
			Name  string  `json:"name"`
			Inner struct1 `json:"inner"`
		}{},
		struct {
			ID    int      `json:"id,omitzero"`
			Name  string   `json:"other_name"`
			Inner *struct1 `json:"inner"`
		}{},
		struct {
			Field  int `json:"field"`
			Ignore int `json:"-"`
		}{},
	}

	for _, c := range cases {
		typ := reflect.TypeOf(c)
		scm, err := typeToSchema(typ, false)
		if err != nil {
			t.Errorf("typeToSchema(%#v) failed with %s", c, err)
		}
		normalizeSchema(scm)

		jscm, err := jsonSchema(typ)
		if err != nil {
			t.Errorf("jsonSchema(%#v) failed with %s", c, err)
		}

		if !reflect.DeepEqual(scm, jscm) {
			buf, _ := json.MarshalIndent(scm, "", "    ")
			jbuf, _ := json.MarshalIndent(jscm, "", "    ")
			t.Errorf("typeToSchema(%#v) got %s want %s", c, string(buf), string(jbuf))
		}
	}
}
