package model

import (
	"testing"
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

/*
func TestToolBuild(t *testing.T) {
	cases := []struct {
		tl   model.Tool
		fail bool
	}{
		{
			tl: model.Tool{
				Name:        "test1",
				Description: "test function 1",
				Args: []model.ToolArg{
					{Name: "s", Description: "string to return"},
				},
				Func: func(s string) (string, error) {
					return s, nil
				},
			},
		},
		{
			tl: model.Tool{
				Name:        "test2",
				Description: "test function 2",
				Args: []model.ToolArg{
					{Name: "i", Description: "an integer"},
					{Name: "f", Description: "a float"},
					{Name: "s", Description: "a string"},
					{Name: "b", Description: "a boolean"},
				},
				Func: func(i int, f float64, s string, b bool) (string, error) {
					return fmt.Sprintf("%d %f %s %v", i, f, s, b), nil
				},
			},
		},
		{
			tl: model.Tool{
				Name:        "test3",
				Description: "test function 3",
				Args: []model.ToolArg{
					{Name: "s", Description: "string to return"},
				},
				Func: func(s string) string {
					return s
				},
			},
			fail: true,
		},
		{
			tl: model.Tool{
				Name:        "test4",
				Description: "test function 4",
				Args: []model.ToolArg{
					{Name: "s", Description: "string to return"},
				},
				Func: func(s string) (string, string, error) {
					return s, s, nil
				},
			},
			fail: true,
		},
		{
			tl: model.Tool{
				Name:        "test5",
				Description: "test function 5",
				Args: []model.ToolArg{
					{Name: "s", Description: "string to return"},
				},
				Func: func(s string) (error, string) {
					return nil, s
				},
			},
			fail: true,
		},
		{
			tl: model.Tool{
				Name:        "test6",
				Description: "test function 6",
				Args: []model.ToolArg{
					{Name: "s1", Description: "string 1"},
					{Name: "s2", Description: "string 2"},
				},
				Func: func(s string) (string, error) {
					return s, nil
				},
			},
			fail: true,
		},
		{
			tl: model.Tool{
				Name:        "test7",
				Description: "test function 7",
				Args: []model.ToolArg{
					{Name: "s1", Description: "string 1"},
					{Name: "s2", Description: "string 2"},
				},
				Func: func(s1, s2, s3 string) (string, error) {
					return s1 + s2 + s3, nil
				},
			},
			fail: true,
		},
		{
			tl: model.Tool{
				Name:        "test8",
				Description: "test function 8",
				Args: []model.ToolArg{
					{Name: "s1", Description: "string 1"},
					{Name: "s2", Description: "string 2"},
				},
				Func: func(s1 string, s2 ...string) (string, error) {
					return s1, nil
				},
			},
			fail: true,
		},
	}

	for _, c := range cases {
		err := c.tl.Build()
		if c.fail {
			if err == nil {
				t.Errorf("Build(%s) did not fail", c.tl.Name)
			}
		} else if err != nil {
			t.Errorf("Build(%s) failed with %s", c.tl.Name, err)
		}
	}
}
*/
