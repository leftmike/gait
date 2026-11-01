package sandbox

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func parseBody(t *testing.T, src string) hcl.Body {
	t.Helper()

	f, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	return f.Body
}

type recordBackend struct {
	name   string
	policy Policy
	body   hcl.Body
}

func (rb *recordBackend) Name() string {
	return rb.name
}

func (rb *recordBackend) Available() error {
	return nil
}

func (rb *recordBackend) New(ctx context.Context, p Policy, cfg hcl.Body) (Sandbox, error) {
	rb.policy = p
	rb.body = cfg
	return nil, nil
}

var recorder = &recordBackend{name: "record"}

func init() {
	Register(recorder)
}

func TestBackends(t *testing.T) {
	if names := Backends(); !slices.Contains(names, "none") ||
		!slices.Contains(names, "record") {

		t.Errorf("Backends = %v", names)
	}
}

func TestOpenBackendBody(t *testing.T) {
	ctx := context.Background()
	p := Policy{Workdir: tempDir(t)}

	if _, err := Open(ctx, "record", p, nil); err != nil {
		t.Fatal(err)
	} else if recorder.body != nil {
		t.Error("body is not nil without a config")
	}

	if _, err := Open(ctx, "record", p, parseBody(t, "none {\n}\n")); err != nil {
		t.Fatal(err)
	} else if recorder.body != nil {
		t.Error("body is not nil without a record block")
	}

	if _, err := Open(ctx, "record", p, parseBody(t, "record {\nx = 1\n}\n")); err != nil {
		t.Fatal(err)
	} else if recorder.body == nil {
		t.Error("body is nil with a record block")
	} else if attrs, _ := recorder.body.JustAttributes(); attrs["x"] == nil {
		t.Error("record block is missing x")
	}

	for _, src := range []string{
		"x = 1\n",
		"unknown {\n}\n",
		"record {\n}\nrecord {\n}\n",
	} {
		if _, err := Open(ctx, "record", p, parseBody(t, src)); err == nil {
			t.Errorf("Open with %q did not fail", src)
		}
	}
}

func TestOpenNormalize(t *testing.T) {
	ctx := context.Background()
	dir := tempDir(t)

	_, err := Open(ctx, "record", Policy{
		Workdir:   dir,
		ReadOnly:  []string{"ro"},
		ReadWrite: []string{"/rw/../x"},
		Deny:      []string{"./deny"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	p := recorder.policy
	if p.Workdir != dir || !slices.Equal(p.ReadOnly, []string{filepath.Join(dir, "ro")}) ||
		!slices.Equal(p.ReadWrite, []string{"/x"}) ||
		!slices.Equal(p.Deny, []string{filepath.Join(dir, "deny")}) {

		t.Errorf("policy = %+v", p)
	}

	_, err = Open(ctx, "record", Policy{
		Workdir: dir,
		Network: Network{Mode: NetAll, Hosts: []string{"example.com"}},
	}, nil)
	if err == nil {
		t.Error("Open with hosts outside the allow list mode did not fail")
	}
}
