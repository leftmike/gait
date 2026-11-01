package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTools(t *testing.T) {
	ctx := context.Background()
	dir := tempDir(t)
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb, err := Open(ctx, "tools", Policy{Workdir: dir, Deny: []string{"./secret"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Close(ctx)

	if CanRun(sb) {
		t.Error("CanRun(tools) = true")
	}
	if _, err := sb.Run(ctx, Cmd{Args: []string{"true"}}); !errors.Is(err, ErrNoCommands) {
		t.Errorf("Run = %v, want ErrNoCommands", err)
	}

	_, err = sb.FS().ReadFile(ctx, secret)
	wantPermission(t, err, "ReadFile of a denied path")
	outside := filepath.Join(tempDir(t), "f")
	if err := os.WriteFile(outside, []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = sb.FS().ReadFile(ctx, outside)
	wantPermission(t, err, "ReadFile outside the policy")
	if err := sb.FS().WriteFile(ctx, filepath.Join(dir, "f"), []byte("y"), 0o644); err != nil {
		t.Errorf("WriteFile in the workdir failed: %s", err)
	}

	_, err = Open(ctx, "tools", Policy{Workdir: dir, Env: []string{"PATH"}}, nil)
	if err == nil {
		t.Error("Open with env succeeded")
	}

	none, err := Open(ctx, "none", Policy{Workdir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !CanRun(none) {
		t.Error("CanRun(none) = false")
	}
}
