package sandbox_test

import (
	"context"
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
