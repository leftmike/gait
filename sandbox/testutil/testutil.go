package testutil

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leftmike/gait/sandbox"
)

type Caps struct {
	Enforced bool
	Network  bool
}

type OpenFunc func(t *testing.T, p sandbox.Policy) sandbox.Sandbox

func tempDir(t *testing.T) string {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func run(t *testing.T, sb sandbox.Sandbox, cmd sandbox.Cmd) sandbox.Result {
	t.Helper()

	res, err := sb.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Run(%q) failed: %s", cmd.Args, err)
	}
	return res
}

func sh(script string) sandbox.Cmd {
	return sandbox.Cmd{Args: []string{"sh", "-c", script}}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func Run(t *testing.T, open OpenFunc, caps Caps) {
	t.Run("ExitCode", func(t *testing.T) {
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		res := run(t, sb, sh("exit 3"))
		if res.ExitCode != 3 {
			t.Errorf("exit code = %d, want 3", res.ExitCode)
		}
	})

	t.Run("Output", func(t *testing.T) {
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		res := run(t, sb, sh("echo out; echo err >&2"))
		if res.ExitCode != 0 {
			t.Errorf("exit code = %d, want 0", res.ExitCode)
		}
		if s := string(res.Output); !strings.Contains(s, "out\n") ||
			!strings.Contains(s, "err\n") {

			t.Errorf("output = %q, want stdout and stderr", s)
		}
	})

	t.Run("MissingCommand", func(t *testing.T) {
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		res, err := sb.Run(context.Background(),
			sandbox.Cmd{Args: []string{"gait-no-such-command"}})
		if err == nil && res.ExitCode == 0 {
			t.Error("missing command succeeded")
		}
	})

	t.Run("EmptyCommand", func(t *testing.T) {
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		if _, err := sb.Run(context.Background(), sandbox.Cmd{}); err == nil {
			t.Error("empty command did not fail")
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		start := time.Now()
		res, err := sb.Run(ctx, sh("sleep 30"))
		if err == nil && res.ExitCode == 0 {
			t.Error("timed out command succeeded")
		}
		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("timed out command took %s", d)
		}
	})

	t.Run("BackgroundChild", func(t *testing.T) {
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		res, err := sb.Run(ctx, sh("sleep 5 & echo done"))
		if err != nil {
			t.Fatal(err)
		}
		if res.ExitCode != 0 || !strings.Contains(string(res.Output), "done") {
			t.Errorf("result = %d %q, want 0 done", res.ExitCode, res.Output)
		}
	})

	t.Run("Workdir", func(t *testing.T) {
		dir := tempDir(t)
		sb := open(t, sandbox.Policy{Workdir: dir})

		res := run(t, sb, sandbox.Cmd{Args: []string{"pwd"}})
		if got := strings.TrimSpace(string(res.Output)); got != dir {
			t.Errorf("pwd = %q, want %q", got, dir)
		}
	})

	t.Run("Dir", func(t *testing.T) {
		dir := tempDir(t)
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		sb := open(t, sandbox.Policy{Workdir: dir})

		for _, d := range []string{sub, "sub"} {
			res := run(t, sb, sandbox.Cmd{Args: []string{"pwd"}, Dir: d})
			if got := strings.TrimSpace(string(res.Output)); got != sub {
				t.Errorf("pwd in %s = %q, want %q", d, got, sub)
			}
		}
	})

	t.Run("Env", func(t *testing.T) {
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		res := run(t, sb, sandbox.Cmd{
			Args: []string{"sh", "-c", "echo $GAIT_TEST_VAR"},
			Env:  map[string]string{"GAIT_TEST_VAR": "value"},
		})
		if got := strings.TrimSpace(string(res.Output)); got != "value" {
			t.Errorf("GAIT_TEST_VAR = %q, want value", got)
		}
	})

	t.Run("SharedFiles", func(t *testing.T) {
		dir := tempDir(t)
		sb := open(t, sandbox.Policy{Workdir: dir})
		ctx := context.Background()

		res := run(t, sb, sh("echo from-cmd > cmd.txt"))
		if res.ExitCode != 0 {
			t.Fatalf("write failed: %s", res.Output)
		}
		data, err := sb.FS().ReadFile(ctx, filepath.Join(dir, "cmd.txt"))
		if err != nil {
			t.Fatal(err)
		} else if string(data) != "from-cmd\n" {
			t.Errorf("cmd.txt = %q, want from-cmd", data)
		}

		err = sb.FS().WriteFile(ctx, filepath.Join(dir, "fs.txt"), []byte("from-fs"), 0o644)
		if err != nil {
			t.Fatal(err)
		}
		res = run(t, sb, sandbox.Cmd{Args: []string{"cat", "fs.txt"}})
		if string(res.Output) != "from-fs" {
			t.Errorf("fs.txt = %q, want from-fs", res.Output)
		}
	})

	if !caps.Enforced {
		return
	}

	t.Run("ReadOnly", func(t *testing.T) {
		ro := tempDir(t)
		writeFile(t, filepath.Join(ro, "f.txt"), "data")
		sb := open(t, sandbox.Policy{Workdir: tempDir(t), ReadOnly: []string{ro}})

		res := run(t, sb, sandbox.Cmd{Args: []string{"cat", filepath.Join(ro, "f.txt")}})
		if res.ExitCode != 0 || string(res.Output) != "data" {
			t.Errorf("read of read-only file = %d %q", res.ExitCode, res.Output)
		}
		res = run(t, sb, sh("echo x > "+filepath.Join(ro, "g.txt")))
		if res.ExitCode == 0 {
			t.Error("write to read-only directory succeeded")
		}

		err := sb.FS().WriteFile(context.Background(), filepath.Join(ro, "g.txt"), nil, 0o644)
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("FS write to read-only directory = %v, want permission error", err)
		}
	})

	t.Run("ReadWrite", func(t *testing.T) {
		rw := tempDir(t)
		sb := open(t, sandbox.Policy{Workdir: tempDir(t), ReadWrite: []string{rw}})

		res := run(t, sb, sh("echo x > "+filepath.Join(rw, "f.txt")))
		if res.ExitCode != 0 {
			t.Errorf("write to read-write directory failed: %s", res.Output)
		}
	})

	t.Run("Deny", func(t *testing.T) {
		dir := tempDir(t)
		secret := filepath.Join(dir, "secret.txt")
		writeFile(t, secret, "secret")
		sb := open(t, sandbox.Policy{Workdir: dir, Deny: []string{secret}})

		res := run(t, sb, sandbox.Cmd{Args: []string{"cat", secret}})
		if strings.Contains(string(res.Output), "secret") {
			t.Errorf("read of denied file = %q", res.Output)
		}

		_, err := sb.FS().ReadFile(context.Background(), secret)
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("FS read of denied file = %v, want permission error", err)
		}
	})

	t.Run("Outside", func(t *testing.T) {
		outside := tempDir(t)
		writeFile(t, filepath.Join(outside, "f.txt"), "outside")
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		res := run(t, sb, sandbox.Cmd{Args: []string{"cat", filepath.Join(outside, "f.txt")}})
		if strings.Contains(string(res.Output), "outside") {
			t.Errorf("read outside the policy = %q", res.Output)
		}
	})

	if !caps.Network {
		return
	}

	t.Run("NetNone", func(t *testing.T) {
		sb := open(t, sandbox.Policy{Workdir: tempDir(t)})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := sb.Run(ctx, sandbox.Cmd{Args: []string{"bash", "-c",
			"exec 3<>/dev/tcp/1.1.1.1/53"}})
		if err == nil && res.ExitCode == 0 {
			t.Error("connection succeeded with no network")
		}
	})
}
