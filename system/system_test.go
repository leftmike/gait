package system

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leftmike/gait/config"
)

func TestDeniedReadWriteError(t *testing.T) {
	sb := checkSandbox(t, &config.SandboxConfig{
		Read:  &config.RWActions{},
		Write: &config.RWActions{},
	}, nil)

	if _, err := sb.ReadFile("/etc/passwd"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("ReadFile denied error = %v, want permission error", err)
	}
	if err := sb.WriteFile("/etc/passwd", nil, 0o644); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("WriteFile denied error = %v, want permission error", err)
	}
}

func TestAskFunc(t *testing.T) {
	cfg := &config.SandboxConfig{Read: &config.RWActions{Ask: []string{"/data/"}}}

	sb := checkSandbox(t, cfg, (&askRecorder{resp: false}).ask)
	if _, err := sb.ReadFile("/data/x.txt"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("ReadFile with declined ask = %v, want permission error", err)
	}

	// When granted, access proceeds to the OS read (which fails because the
	// file does not exist, not with a permission error).
	sb = checkSandbox(t, cfg, (&askRecorder{resp: true}).ask)
	if _, err := sb.ReadFile("/data/does-not-exist.txt"); errors.Is(err, fs.ErrPermission) {
		t.Errorf("ReadFile with granted ask returned permission error: %v", err)
	}
}

func TestNoAskFunc(t *testing.T) {
	// A nil ask func denies the operations that would ask, rather than
	// panicking on a nil call.
	sb := checkSandbox(t, &config.SandboxConfig{
		Read:  &config.RWActions{Ask: []string{"/data/"}},
		Write: &config.RWActions{Ask: []string{"/data/"}},
	}, nil)
	wantPathError(t, sb.checkRW(sb.readActions, "read", "/data/x.txt"), "read", "/data/x.txt")
	wantPathError(t, sb.checkRW(sb.writeActions, "write", "/data/x.txt"), "write", "/data/x.txt")

	// A nil ask func with nothing to ask about is still unrestricted.
	sb = checkSandbox(t, nil, nil)
	if err := sb.checkRW(sb.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW read on an unrestricted sandbox failed: %s", err)
	}
}

func TestSandboxAskFuncIndependent(t *testing.T) {
	// Each sandbox carries its own ask func; two built from the same config
	// answer independently.
	cfg := &config.SandboxConfig{Read: &config.RWActions{Ask: []string{"/data/"}}}
	allowAsk, denyAsk := &askRecorder{resp: true}, &askRecorder{resp: false}
	allowSB := checkSandbox(t, cfg, allowAsk.ask)
	denySB := checkSandbox(t, cfg, denyAsk.ask)

	if err := allowSB.checkRW(allowSB.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW read on the allowing sandbox failed: %s", err)
	}
	wantPathError(t, denySB.checkRW(denySB.readActions, "read", "/data/x.txt"), "read",
		"/data/x.txt")

	if allowAsk.calls != 1 || denyAsk.calls != 1 {
		t.Errorf("ask calls = %d and %d, want 1 each", allowAsk.calls, denyAsk.calls)
	}
}

func (a action) String() string {
	switch a {
	case allow:
		return "allow"
	case ask:
		return "ask"
	default:
		return "deny"
	}
}

func TestAddPaths(t *testing.T) {
	var rwa rwActions

	err := rwa.addPaths([]string{"/etc/hosts", "/home/mike/", "/a/b/c.txt"}, allow)
	if err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	err = rwa.addPaths([]string{"/home/mike/.ssh/", "/etc/shadow"}, deny)
	if err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}

	wantFiles := map[string]action{
		"/etc/hosts":  allow,
		"/a/b/c.txt":  allow,
		"/etc/shadow": deny,
	}
	if !maps.Equal(rwa.files, wantFiles) {
		t.Errorf("addPaths files = %v, want %v", rwa.files, wantFiles)
	}

	wantDirs := []dirAction{{"/home/mike/", allow}, {"/home/mike/.ssh/", deny}}
	if !slices.Equal(rwa.dirs, wantDirs) {
		t.Errorf("addPaths dirs = %v, want %v", rwa.dirs, wantDirs)
	}
}

func TestAddPathsEmpty(t *testing.T) {
	// Only directories: the files map is never allocated, and looking a path up
	// in the nil map must not panic.
	var rwa rwActions
	if err := rwa.addPaths([]string{"/data/"}, allow); err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	if rwa.files != nil {
		t.Errorf("addPaths files = %v, want nil", rwa.files)
	}
	if got := rwa.pathAction("/data/x.txt", deny); got != allow {
		t.Errorf("pathAction(/data/x.txt) = %s, want allow", got)
	}

	// No paths at all is not an error.
	if err := rwa.addPaths(nil, allow); err != nil {
		t.Errorf("addPaths(nil) failed: %s", err)
	}
}

func TestAddPathsDuplicateFile(t *testing.T) {
	var rwa rwActions
	if err := rwa.addPaths([]string{"/a/b"}, allow); err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	if err := rwa.addPaths([]string{"/a/b"}, deny); err == nil {
		t.Error("addPaths with duplicate file path did not fail")
	}

	// Within a single call as well.
	var rwa2 rwActions
	if err := rwa2.addPaths([]string{"/a/b", "/a/b"}, allow); err == nil {
		t.Error("addPaths with repeated file path did not fail")
	}
}

func TestNewRWActionsNil(t *testing.T) {
	rwa, err := newRWActions(nil)
	if err != nil {
		t.Fatalf("newRWActions(nil) failed: %s", err)
	}
	if rwa != nil {
		t.Fatalf("newRWActions(nil) = %v, want nil", rwa)
	}
	// A nil *rwActions always yields the default.
	for _, dflt := range []action{deny, ask, allow} {
		if got := rwa.pathAction("/anything", dflt); got != dflt {
			t.Errorf("nil pathAction(/anything, %s) = %s, want %s", dflt, got, dflt)
		}
	}
}

func TestNewRWActionsEmpty(t *testing.T) {
	rwa, err := newRWActions(&config.RWActions{})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	if rwa == nil {
		t.Fatal("newRWActions(&config.RWActions{}) = nil, want non-nil")
	}
	if got := rwa.pathAction("/anything", ask); got != ask {
		t.Errorf("pathAction with no paths = %s, want ask", got)
	}
}

func TestPathAction(t *testing.T) {
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/home/mike/", "/etc/hosts"},
		Ask:   []string{"/home/mike/secrets/", "/tmp/scratch"},
		Deny:  []string{"/home/mike/.ssh"},
	})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	cases := []struct {
		path string
		want action
	}{
		{"/etc/hosts", allow},               // exact file allow
		{"/tmp/scratch", ask},               // exact file ask
		{"/home/mike/.ssh", deny},           // exact file deny
		{"/home/mike/notes.txt", allow},     // beneath the allowed directory
		{"/home/mike/sub/notes.txt", allow}, // deeper beneath it
		{"/home/mike/.ssh/config", allow},   // a file rule does not cover children
		{"/etc/hostsX", deny},               // no prefix matching on a file rule
		{"/var/log/messages", deny},         // nothing matches -> default
		{"/home/mike", allow},               // the directory itself, so it can be made & removed
		{"/home/mikex", deny},               // but not a sibling sharing the prefix
		{"/home/mike/secrets/keys", ask},    // more specific ask beats the allow
		{"/home/mike/secrets/a/b.txt", ask}, // deeper beneath the ask directory
	}

	for _, c := range cases {
		if got := rwa.pathAction(c.path, deny); got != c.want {
			t.Errorf("pathAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestPathActionClean(t *testing.T) {
	// Both the configured paths and the path under test are cleaned before
	// matching, so "." and ".." elements cannot slip past a rule.
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/home/mike/"},
		Deny:  []string{"/home/mike/./secrets/keys/", "/home/mike/x/../.ssh"},
	})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	wantDirs := []dirAction{{"/home/mike/secrets/keys/", deny}, {"/home/mike/", allow}}
	if !slices.Equal(rwa.dirs, wantDirs) {
		t.Errorf("dirs = %v, want %v", rwa.dirs, wantDirs)
	}

	cases := []struct {
		path string
		want action
	}{
		{"/home/mike/../mike/notes.txt", allow},
		{"/home/mike/./notes.txt", allow},
		{"/home/mike/secrets/keys/id_rsa", deny},
		{"/home/mike/../mike/secrets/keys/id_rsa", deny},
		{"/home/mike/secrets/../secrets/keys/id_rsa", deny},
		{"/home/mike/.ssh", deny},
		{"/home/mike/a/b/../../.ssh", deny},
	}

	for _, c := range cases {
		if got := rwa.pathAction(c.path, deny); got != c.want {
			t.Errorf("pathAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestPathActionRootDir(t *testing.T) {
	// Cleaning "/" must not turn it into "//".
	rwa, err := newRWActions(&config.RWActions{Allow: []string{"/"}})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	if wantDirs := []dirAction{{"/", allow}}; !slices.Equal(rwa.dirs, wantDirs) {
		t.Fatalf("dirs = %v, want %v", rwa.dirs, wantDirs)
	}
	for _, p := range []string{"/anything", "/a/b/c"} {
		if got := rwa.pathAction(p, deny); got != allow {
			t.Errorf("pathAction(%q) = %s, want allow", p, got)
		}
	}
}

func TestPathActionFileBeatsDir(t *testing.T) {
	// A file entry is consulted before any directory entry, whatever the
	// directory's action.
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/data/"},
		Deny:  []string{"/data/secret.txt"},
	})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	if got := rwa.pathAction("/data/secret.txt", allow); got != deny {
		t.Errorf("pathAction(/data/secret.txt) = %s, want deny", got)
	}
	if got := rwa.pathAction("/data/other.txt", deny); got != allow {
		t.Errorf("pathAction(/data/other.txt) = %s, want allow", got)
	}
}

func TestPathActionDefault(t *testing.T) {
	rwa, err := newRWActions(&config.RWActions{Allow: []string{"/data/"}})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	// An unmatched path takes the default, whatever it is.
	for _, dflt := range []action{deny, ask, allow} {
		if got := rwa.pathAction("/elsewhere/x", dflt); got != dflt {
			t.Errorf("pathAction(/elsewhere/x, %s) = %s, want %s", dflt, got, dflt)
		}
	}
}

func TestNewRWActionsDuplicateFile(t *testing.T) {
	_, err := newRWActions(&config.RWActions{
		Allow: []string{"/a/b"},
		Deny:  []string{"/a/b"},
	})
	if err == nil {
		t.Error("newRWActions with a duplicate file path did not fail")
	}
}

func TestNewRWActionsDuplicateDir(t *testing.T) {
	_, err := newRWActions(&config.RWActions{
		Allow: []string{"/a/b/"},
		Deny:  []string{"/a/b/"},
	})
	if err == nil {
		t.Error("newRWActions with a duplicate directory did not fail")
	}

	// The duplicate is not adjacent in the order the paths are added, so
	// finding it depends on the directories being sorted first.
	_, err = newRWActions(&config.RWActions{
		Allow: []string{"/a/b/", "/c/d/"},
		Deny:  []string{"/a/b/"},
	})
	if err == nil {
		t.Error("newRWActions with a non-adjacent duplicate directory did not fail")
	}
}

func TestPathActionMostSpecificDir(t *testing.T) {
	// Nested directories: the longest matching directory determines the
	// action, regardless of which config list it came from.
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/home/mike/"},
		Ask:   []string{"/home/mike/secrets/"},
		Deny:  []string{"/home/mike/secrets/keys/"},
	})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	cases := []struct {
		path string
		want action
	}{
		{"/home/mike/notes.txt", allow},
		{"/home/mike/secrets/a.txt", ask},
		{"/home/mike/secrets/keys/id_rsa", deny},
	}

	for _, c := range cases {
		if got := rwa.pathAction(c.path, deny); got != c.want {
			t.Errorf("pathAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

// askRecorder is an AskFunc that always answers resp, recording the calls made
// and the arguments of the last one.
type askRecorder struct {
	resp     bool
	calls    int
	lastOp   string
	lastPath string
}

func (ar *askRecorder) ask(op, path string) bool {
	ar.calls += 1
	ar.lastOp, ar.lastPath = op, path
	return ar.resp
}

// wantPathError checks that err is a *fs.PathError carrying op, path and a
// permission error.
func wantPathError(t *testing.T, err error, op, path string) {
	t.Helper()

	var perr *fs.PathError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *fs.PathError", err)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error = %v, want permission error", err)
	}
	if perr.Op != op {
		t.Errorf("error Op = %q, want %q", perr.Op, op)
	}
	if perr.Path != path {
		t.Errorf("error Path = %q, want %q", perr.Path, path)
	}
}

func checkSandbox(t *testing.T, cfg *config.SandboxConfig, ask AskFunc) *Sandbox {
	t.Helper()

	sb, err := NewSandbox(cfg, ask)
	if err != nil {
		t.Fatalf("NewSandbox failed: %s", err)
	}
	return sb
}

func TestCheckRW(t *testing.T) {
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read:  &config.RWActions{Allow: []string{"/data/", "/etc/hosts"}},
			Write: &config.RWActions{Allow: []string{"/data/out/"}},
		}, nil)

	// Reads are governed by the read actions.
	if err := sb.checkRW(sb.readActions, "read", "/data/in.txt"); err != nil {
		t.Errorf("checkRW(read, /data/in.txt) failed: %s", err)
	}
	if err := sb.checkRW(sb.readActions, "read", "/etc/hosts"); err != nil {
		t.Errorf("checkRW(read, /etc/hosts) failed: %s", err)
	}
	wantPathError(t, sb.checkRW(sb.readActions, "read", "/etc/shadow"), "read", "/etc/shadow")

	// Writes are governed by the write actions, which are separate: /data/ is
	// readable but only /data/out/ is writable.
	if err := sb.checkRW(sb.writeActions, "write", "/data/out/x.txt"); err != nil {
		t.Errorf("checkRW(write, /data/out/x.txt) failed: %s", err)
	}
	wantPathError(t, sb.checkRW(sb.writeActions, "write", "/data/in.txt"), "write", "/data/in.txt")
	wantPathError(t, sb.checkRW(sb.writeActions, "write", "/etc/hosts"), "write", "/etc/hosts")
}

func TestCheckRWAsk(t *testing.T) {
	cfg := &config.SandboxConfig{
		Read:  &config.RWActions{Ask: []string{"/data/"}},
		Write: &config.RWActions{Ask: []string{"/data/"}},
	}

	// Granted: no error, and the op and path are passed through to the ask func.
	ar := &askRecorder{resp: true}
	sb := checkSandbox(t, cfg, ar.ask)
	if err := sb.checkRW(sb.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW read with granted ask failed: %s", err)
	}
	if ar.calls != 1 {
		t.Errorf("ask func called %d times, want 1", ar.calls)
	}
	if ar.lastOp != "read" || ar.lastPath != "/data/x.txt" {
		t.Errorf("ask func(%q, %q), want (%q, %q)", ar.lastOp, ar.lastPath, "read",
			"/data/x.txt")
	}

	if err := sb.checkRW(sb.writeActions, "write", "/data/x.txt"); err != nil {
		t.Errorf("checkRW write with granted ask failed: %s", err)
	}
	if ar.lastOp != "write" {
		t.Errorf("ask func op = %q, want %q", ar.lastOp, "write")
	}

	// Declined: a permission error.
	sb = checkSandbox(t, cfg, (&askRecorder{resp: false}).ask)
	wantPathError(t, sb.checkRW(sb.readActions, "read", "/data/x.txt"), "read", "/data/x.txt")
	wantPathError(t, sb.checkRW(sb.writeActions, "write", "/data/x.txt"), "write", "/data/x.txt")
}

func TestCheckRWNoAsk(t *testing.T) {
	// The ask func must only be consulted for an ask action, never for allow or
	// for a denied path.
	ar := &askRecorder{resp: true}
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read:  &config.RWActions{Allow: []string{"/data/"}, Deny: []string{"/data/no/"}},
			Write: &config.RWActions{Allow: []string{"/data/"}, Deny: []string{"/data/no/"}},
		}, ar.ask)

	if err := sb.checkRW(sb.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW(read, /data/x.txt) failed: %s", err)
	}
	wantPathError(t, sb.checkRW(sb.readActions, "read", "/data/no/x.txt"), "read",
		"/data/no/x.txt")
	wantPathError(t, sb.checkRW(sb.writeActions, "write", "/elsewhere"), "write", "/elsewhere")

	if ar.calls != 0 {
		t.Errorf("ask func called %d times, want 0", ar.calls)
	}
}

func TestCheckRWUncleanPath(t *testing.T) {
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read:  &config.RWActions{Allow: []string{"/data/"}, Deny: []string{"/data/no/"}},
			Write: &config.RWActions{Allow: []string{"/data/"}, Deny: []string{"/data/no/"}},
		}, nil)

	// The path is cleaned before matching, so ".." cannot escape a deny.
	const unclean = "/data/yes/../no/x.txt"
	wantPathError(t, sb.checkRW(sb.readActions, "read", unclean), "read", unclean)
	wantPathError(t, sb.checkRW(sb.writeActions, "write", unclean), "write", unclean)

	// An unclean path that lands somewhere allowed is still allowed.
	if err := sb.checkRW(sb.readActions, "read", "/data/./a/../x.txt"); err != nil {
		t.Errorf("checkRW(read, /data/./a/../x.txt) failed: %s", err)
	}
}

func TestCheckRWMissingConfig(t *testing.T) {
	// Within a configured sandbox, a config block that is absent leaves its
	// actions nil and denies everything.
	sb := checkSandbox(t, &config.SandboxConfig{
		Read: &config.RWActions{Allow: []string{"/data/"}},
	}, nil)
	if sb.writeActions != nil {
		t.Fatalf("writeActions = %v, want nil", sb.writeActions)
	}
	if err := sb.checkRW(sb.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW(read, /data/x.txt) failed: %s", err)
	}
	wantPathError(t, sb.checkRW(sb.writeActions, "write", "/data/x.txt"), "write", "/data/x.txt")

	// A config block with no paths in it denies everything too.
	sb = checkSandbox(t, &config.SandboxConfig{
		Read:  &config.RWActions{},
		Write: &config.RWActions{},
	}, nil)
	wantPathError(t, sb.checkRW(sb.readActions, "read", "/anything"), "read", "/anything")
	wantPathError(t, sb.checkRW(sb.writeActions, "write", "/anything"), "write", "/anything")
}

func TestCheckRWNoSandbox(t *testing.T) {
	// No sandbox configured at all: unrestricted access, and the user is never
	// prompted.
	ar := &askRecorder{resp: false}
	sb := checkSandbox(t, nil, ar.ask)

	for _, p := range []string{"/anything", "/etc/shadow", "relative/path"} {
		if err := sb.checkRW(sb.readActions, "read", p); err != nil {
			t.Errorf("checkRW(read, %q) failed: %s", p, err)
		}
		if err := sb.checkRW(sb.writeActions, "write", p); err != nil {
			t.Errorf("checkRW(write, %q) failed: %s", p, err)
		}
	}
	if ar.calls != 0 {
		t.Errorf("ask func called %d times, want 0", ar.calls)
	}
}

func TestMkdirAllRemove(t *testing.T) {
	dir := t.TempDir()
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Write: &config.RWActions{Allow: []string{dir + "/"}},
		}, nil)

	// Making and removing a directory within the writable directory.
	sub := filepath.Join(dir, "sub")
	if err := sb.MkdirAll(sub, 0o755); err != nil {
		t.Errorf("MkdirAll(%q) failed: %s", sub, err)
	}
	if err := sb.Remove(sub); err != nil {
		t.Errorf("Remove(%q) failed: %s", sub, err)
	}

	// Neither is allowed outside of it.
	outside := filepath.Join(filepath.Dir(dir), "outside")
	wantPathError(t, sb.MkdirAll(outside, 0o755), "mkdir", outside)
	wantPathError(t, sb.Remove(outside), "remove", outside)

	// Writable does not imply readable.
	if _, err := sb.Stat(sub); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("Stat(%q) = %v, want permission error", sub, err)
	}
}

func TestStat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %s", path, err)
	}

	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read: &config.RWActions{
				Allow: []string{dir + "/"},
				Deny:  []string{filepath.Join(dir, "no") + "/"},
			},
		}, nil)

	fi, err := sb.Stat(path)
	if err != nil {
		t.Errorf("Stat(%q) failed: %s", path, err)
	} else if fi.Name() != "x.txt" {
		t.Errorf("Stat(%q) name = %q, want %q", path, fi.Name(), "x.txt")
	}

	denied := filepath.Join(dir, "no", "x.txt")
	_, err = sb.Stat(denied)
	wantPathError(t, err, "stat", denied)
}

// walkTree creates root/a.txt, root/no/b.txt and root/sub/c.txt.
func walkTree(t *testing.T, root string) {
	t.Helper()

	for _, p := range []string{"a.txt", "no/b.txt", "sub/c.txt"} {
		path := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %s", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %s", path, err)
		}
	}
}

func walkPaths(t *testing.T, sb *Sandbox, root string) []string {
	t.Helper()

	var paths []string
	err := sb.WalkDir(root,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			paths = append(paths, filepath.ToSlash(rel))
			return nil
		})
	if err != nil {
		t.Fatalf("WalkDir(%q) failed: %s", root, err)
	}

	slices.Sort(paths)
	return paths
}

func TestWalkDirDenied(t *testing.T) {
	root := t.TempDir()
	walkTree(t, root)

	// A denied directory is skipped entirely: neither it nor anything within it
	// is walked, and the rest of the walk continues.
	ar := &askRecorder{resp: true}
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read: &config.RWActions{
				Allow: []string{root + "/"},
				Deny:  []string{filepath.Join(root, "no") + "/"},
			},
		}, ar.ask)

	got := walkPaths(t, sb, root)
	want := []string{".", "a.txt", "sub", "sub/c.txt"}
	if !slices.Equal(got, want) {
		t.Errorf("WalkDir(%q) = %v, want %v", root, got, want)
	}
	if ar.calls != 0 {
		t.Errorf("ask func called %d times, want 0", ar.calls)
	}
}

func TestWalkDirDeniedRoot(t *testing.T) {
	root := t.TempDir()
	walkTree(t, root)

	// The root is checked up front; a denied root fails the walk.
	sb := checkSandbox(t, &config.SandboxConfig{Read: &config.RWActions{}}, nil)
	var walked bool
	err := sb.WalkDir(root,
		func(path string, d fs.DirEntry, err error) error {
			walked = true
			return nil
		})
	wantPathError(t, err, "walkdir", root)
	if walked {
		t.Errorf("WalkDir(%q) called the walk func for a denied root", root)
	}
}

func TestWalkDirAsk(t *testing.T) {
	root := t.TempDir()
	walkTree(t, root)

	// The root is asked about once; the entries below it are then walked
	// without asking about each one.
	ar := &askRecorder{resp: true}
	sb := checkSandbox(t,
		&config.SandboxConfig{Read: &config.RWActions{Ask: []string{root + "/"}}}, ar.ask)

	got := walkPaths(t, sb, root)
	want := []string{".", "a.txt", "no", "no/b.txt", "sub", "sub/c.txt"}
	if !slices.Equal(got, want) {
		t.Errorf("WalkDir(%q) = %v, want %v", root, got, want)
	}
	if ar.calls != 1 {
		t.Errorf("ask func called %d times, want 1", ar.calls)
	}
	if ar.lastOp != "walkdir" || ar.lastPath != root {
		t.Errorf("ask func(%q, %q), want (%q, %q)", ar.lastOp, ar.lastPath, "walkdir", root)
	}
}
