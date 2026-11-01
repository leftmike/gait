package sandbox_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/leftmike/gait/sandbox"
	"github.com/leftmike/gait/sandbox/testutil"
)

func TestNone(t *testing.T) {
	testutil.Run(t, func(t *testing.T, p sandbox.Policy) sandbox.Sandbox {
		sb, err := sandbox.Open(context.Background(), "none", p, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { sb.Close(context.Background()) })
		return sb
	}, testutil.Caps{})
}

func TestOpenUnknown(t *testing.T) {
	_, err := sandbox.Open(context.Background(), "no-such-backend", sandbox.Policy{}, nil)
	if err == nil {
		t.Error("Open of an unknown backend succeeded")
	}
}

func TestNono(t *testing.T) {
	if _, err := exec.LookPath("nono"); err != nil {
		t.Skip("nono not installed")
	}

	testutil.Run(t, func(t *testing.T, p sandbox.Policy) sandbox.Sandbox {
		sb, err := sandbox.Open(context.Background(), "nono", p, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { sb.Close(context.Background()) })
		return sb
	}, testutil.Caps{Enforced: true, Network: true})
}
