package tool

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/invopop/jsonschema"
)

func callJSON(t *testing.T, fn func(context.Context, []byte) (string, error), v any) string {
	t.Helper()
	buf, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	out, err := fn(context.Background(), buf)
	if err != nil {
		t.Fatalf("tool returned error: %s", err)
	}
	return out
}

func callJSONErr(t *testing.T, fn func(context.Context, []byte) (string, error), v any) {
	t.Helper()
	buf, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fn(context.Background(), buf); err == nil {
		t.Fatalf("expected error, got none")
	}
}

// TestToolsNoPanic ensures every file tool's schema builds (MustToolSchema
// panics on a bad gait tag, e.g. a comma in a description).
func TestToolsNoPanic(t *testing.T) {
	tools := map[string]Tool{}
	for _, tl := range []Tool{ReadFile, WriteFile, EditFile, Glob, Grep, Bash} {
		tools[tl.Name] = tl
	}
	for _, name := range []string{"read_file", "write_file", "edit_file", "glob", "grep", "bash"} {
		if _, ok := tools[name]; !ok {
			t.Errorf("tool %q not registered", name)
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
	r := jsonschema.Reflector{
		DoNotReference:            true,
		AllowAdditionalProperties: true,
	}
	sc := r.ReflectFromType(typ)

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
			delete(scm, "$schema")
		})
	return scm, nil
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
			ID    int      `json:"id,omitempty"`
			Name  string   `json:"other_name"`
			Inner *struct1 `json:"inner"`
		}{},
		struct {
			Field  int `json:"field"`
			ID     *int
			Ignore int            `json:"-"`
			Dict   map[string]int `json:"dict"`
		}{},
	}

	for _, c := range cases {
		typ := reflect.TypeOf(c)
		scm, err := typeToSchema(typ)
		if err != nil {
			t.Errorf("typeToSchema(%#v) failed with %s", c, err)
		}

		jscm, err := jsonSchema(typ)
		if err != nil {
			t.Errorf("jsonSchema(%#v) failed with %s", c, err)
		}

		if !reflect.DeepEqual(scm, jscm) {
			buf, _ := json.MarshalIndent(scm, "", "    ")
			jbuf, _ := json.MarshalIndent(jscm, "", "    ")
			t.Errorf("typeToSchema(%#v)\ngot %s\nwant %s", c, string(buf), string(jbuf))
		}
	}
}
