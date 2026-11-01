//go:build microsandbox

package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/leftmike/gait/sandbox"
	"github.com/leftmike/gait/sandbox/testutil"
)

func TestMicrosandbox(t *testing.T) {
	b, _ := sandbox.Lookup("microsandbox")
	if err := b.Available(); err != nil {
		t.Skip(err)
	}

	f, diags := hclsyntax.ParseConfig([]byte("microsandbox {\nimage = \"alpine\"\n}\n"),
		"test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags)
	}

	testutil.Run(t, func(t *testing.T, p sandbox.Policy) sandbox.Sandbox {
		sb, err := sandbox.Open(context.Background(), "microsandbox", p, f.Body)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { sb.Close(context.Background()) })
		return sb
	}, testutil.Caps{Enforced: true, NestedDeny: true, Network: true})
}

func TestMicrosandboxClose(t *testing.T) {
	b, _ := sandbox.Lookup("microsandbox")
	if err := b.Available(); err != nil {
		t.Skip(err)
	}

	f, diags := hclsyntax.ParseConfig([]byte("microsandbox {\nimage = \"alpine\"\n}\n"),
		"test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags)
	}

	ctx := context.Background()
	sb, err := sandbox.Open(ctx, "microsandbox", sandbox.Policy{Workdir: t.TempDir()}, f.Body)
	if err != nil {
		t.Fatal(err)
	}
	tctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if res, err := sb.Run(tctx, sandbox.Cmd{Args: []string{"sleep", "30"}}); err != nil ||
		res.ExitCode != -1 {

		t.Errorf("Run during boot = %d %v, want -1", res.ExitCode, err)
	}
	if res, err := sb.Run(ctx, sandbox.Cmd{Args: []string{"true"}}); err != nil ||
		res.ExitCode != 0 {

		t.Errorf("Run after a timeout = %d %v", res.ExitCode, err)
	}
	if err := sb.Close(ctx); err != nil {
		t.Errorf("Close = %v", err)
	}
}
