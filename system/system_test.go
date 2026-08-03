package system

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	wantPathError(t, checkRW(sb, sb.readActions, "read", "/data/x.txt"), "read", "/data/x.txt")
	wantPathError(t, checkRW(sb, sb.writeActions, "write", "/data/x.txt"), "write", "/data/x.txt")

	// A nil ask func with nothing to ask about is still unrestricted.
	sb = checkSandbox(t, nil, nil)
	if err := checkRW(sb, sb.readActions, "read", "/data/x.txt"); err != nil {
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

	if err := checkRW(allowSB, allowSB.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW read on the allowing sandbox failed: %s", err)
	}
	wantPathError(t, checkRW(denySB, denySB.readActions, "read", "/data/x.txt"), "read",
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

// checkExpandPath expands path and compares the result with what is wanted.
func checkExpandPath(t *testing.T, path, cwd, home string, look bool, wantPath string,
	wantDir bool) {

	t.Helper()

	gotPath, gotDir, err := expandPath(path, cwd, home, look)
	if err != nil {
		t.Errorf("expandPath(%q) failed: %s", path, err)
		return
	}
	if gotPath != wantPath || gotDir != wantDir {
		t.Errorf("expandPath(%q) = %q, %v; want %q, %v", path, gotPath, gotDir, wantPath,
			wantDir)
	}
}

func TestExpandPathAbsolute(t *testing.T) {
	// An absolute path is used as given, cleaned; cwd and home are not consulted.
	path := testPath(t)
	cwd, home := t.TempDir(), t.TempDir()

	cases := []struct {
		path    string
		want    string
		wantDir bool
	}{
		{path("etc/hosts"), path("etc/hosts"), false},
		{path("etc/hosts/"), path("etc/hosts"), true}, // trailing / on a missing path
		{path("etc/./hosts"), path("etc/hosts"), false},
		{path("etc/x/../hosts"), path("etc/hosts"), false},
		{path("etc//hosts"), path("etc/hosts"), false},
		{path("etc/hosts//"), path("etc/hosts"), true},
		{"/", "/", true}, // cleaning / must not produce //
	}

	for _, c := range cases {
		checkExpandPath(t, c.path, cwd, home, false, c.want, c.wantDir)
		// look only affects bare names, never absolute paths.
		checkExpandPath(t, c.path, cwd, home, true, c.want, c.wantDir)
	}
}

func TestExpandPathCwd(t *testing.T) {
	// "." and "./" are relative to cwd.
	cwd, home := t.TempDir(), t.TempDir()
	sub := filepath.Join(cwd, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) failed: %s", sub, err)
	}
	file := filepath.Join(cwd, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %s", file, err)
	}

	cases := []struct {
		path    string
		want    string
		wantDir bool
	}{
		{".", cwd, true},
		{"./", cwd, true},
		{"./sub", sub, true},  // a directory on disk, without a trailing /
		{"./sub/", sub, true}, // and with one
		{"./file.txt", file, false},
		{"./missing", filepath.Join(cwd, "missing"), false},
		{"./missing/", filepath.Join(cwd, "missing"), true},
		{"./a/../b", filepath.Join(cwd, "b"), false},
		{"./sub/./x", filepath.Join(sub, "x"), false},
		{"./sub/../sub/x", filepath.Join(sub, "x"), false},
		// "./.." escapes cwd; expansion does not prevent that.
		{"./..", filepath.Dir(cwd), true},
	}

	for _, c := range cases {
		checkExpandPath(t, c.path, cwd, home, false, c.want, c.wantDir)
		checkExpandPath(t, c.path, cwd, home, true, c.want, c.wantDir)
	}
}

func TestExpandPathHome(t *testing.T) {
	// "~" and "~/" are relative to home, not to cwd.
	cwd, home := t.TempDir(), t.TempDir()
	sub := filepath.Join(home, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) failed: %s", sub, err)
	}
	file := filepath.Join(home, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %s", file, err)
	}
	// A decoy of the same name in cwd, so that expanding relative to the wrong
	// base is visible in the result.
	if err := os.WriteFile(filepath.Join(cwd, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile in cwd failed: %s", err)
	}

	cases := []struct {
		path    string
		want    string
		wantDir bool
	}{
		{"~", home, true},
		{"~/", home, true},
		{"~/sub", sub, true},
		{"~/sub/", sub, true},
		{"~/file.txt", file, false},
		{"~/missing", filepath.Join(home, "missing"), false},
		{"~/missing/", filepath.Join(home, "missing"), true},
		{"~/a/../b", filepath.Join(home, "b"), false},
	}

	for _, c := range cases {
		checkExpandPath(t, c.path, cwd, home, false, c.want, c.wantDir)
		checkExpandPath(t, c.path, cwd, home, true, c.want, c.wantDir)
	}
}

func TestExpandPathExisting(t *testing.T) {
	// What exists on disk decides isDir: a directory is a directory rule however
	// it is written, and a file with a trailing / is a mistake.
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) failed: %s", sub, err)
	}
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %s", file, err)
	}

	dirLink := filepath.Join(root, "dirlink")
	if err := os.Symlink(sub, dirLink); err != nil {
		t.Fatalf("Symlink(%q) failed: %s", dirLink, err)
	}
	fileLink := filepath.Join(root, "filelink")
	if err := os.Symlink(file, fileLink); err != nil {
		t.Fatalf("Symlink(%q) failed: %s", fileLink, err)
	}

	cases := []struct {
		path    string
		want    string
		wantDir bool
	}{
		{root, root, true},
		{sub, sub, true},
		{file, file, false},
		// Symlinks are resolved, so that what is checked is what the operation
		// will reach; a link to a directory is still a directory.
		{dirLink, sub, true},
		{dirLink + "/", sub, true},
		{fileLink, file, false},
		// A file rule does not cover children, and a missing child of a file is
		// not an error here.
		{filepath.Join(file, "child"), filepath.Join(file, "child"), false},
	}

	for _, c := range cases {
		checkExpandPath(t, c.path, root, root, false, c.want, c.wantDir)
	}

	// A trailing / on something which exists and is not a directory is an error.
	for _, p := range []string{file + "/", fileLink + "/"} {
		if _, _, err := expandPath(p, root, root, false); err == nil {
			t.Errorf("expandPath(%q) did not fail for a file with a trailing /", p)
		}
	}
}

func TestExpandPathErrors(t *testing.T) {
	cwd, home := t.TempDir(), t.TempDir()

	// A relative path must start with "./"; a bare name is only looked up when
	// look is set, and never when it contains a "/".
	for _, path := range []string{"", "foo", "a/b", "sub/", "..", "../a", "~x", "~user/x",
		".hidden"} {

		if _, _, err := expandPath(path, cwd, home, false); err == nil {
			t.Errorf("expandPath(%q) did not fail", path)
		}
	}

	// Even with look set, anything containing a "/" is rejected rather than
	// looked up.
	for _, path := range []string{"a/b", "sub/", "../a", "~user/x"} {
		if _, _, err := expandPath(path, cwd, home, true); err == nil {
			t.Errorf("expandPath(%q) with look did not fail", path)
		}
	}
}

func TestExpandPathLook(t *testing.T) {
	// A bare name is resolved on PATH, but only when look is set.
	dir := t.TempDir()
	cmd := makeExec(t, dir, "gaitcmd")
	t.Setenv("PATH", dir)

	checkExpandPath(t, "gaitcmd", t.TempDir(), t.TempDir(), true, cmd, false)

	if _, _, err := expandPath("gaitcmd", dir, dir, false); err == nil {
		t.Errorf("expandPath(%q) without look did not fail", "gaitcmd")
	}
	if _, _, err := expandPath("no-such-command", dir, dir, true); err == nil {
		t.Errorf("expandPath of a command which is not on PATH did not fail")
	}
}

func TestAddPathsExpands(t *testing.T) {
	// The paths in the configuration go through expandPath, so "./" and "~/"
	// work there.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := t.TempDir()
	t.Chdir(cwd)
	// t.Chdir may land on a path which differs from cwd by a symlink, and
	// expandPath uses what os.Getwd reports.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %s", err)
	}

	var rwa rwActions
	err = rwa.addPaths([]string{"./notes.txt", "~/.ssh/", "./data/"}, allow, cwd, home)
	if err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}

	wantFiles := map[string]action{filepath.Join(cwd, "notes.txt"): allow}
	if !maps.Equal(rwa.files, wantFiles) {
		t.Errorf("addPaths files = %v, want %v", rwa.files, wantFiles)
	}
	wantDirs := []dirAction{
		{filepath.Join(home, ".ssh") + "/", allow},
		{filepath.Join(cwd, "data") + "/", allow},
	}
	if !slices.Equal(rwa.dirs, wantDirs) {
		t.Errorf("addPaths dirs = %v, want %v", rwa.dirs, wantDirs)
	}

	// Commands are not looked up on PATH for read and write rules.
	if err := rwa.addPaths([]string{"sh"}, allow, cwd, home); err == nil {
		t.Errorf("addPaths of a bare name did not fail")
	}
}

func TestAddPaths(t *testing.T) {
	var rwa rwActions

	err := rwa.addPaths([]string{"/etc/hosts", "/home/mike/", "/a/b/c.txt"}, allow, "", "")
	if err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	err = rwa.addPaths([]string{"/home/mike/.ssh/", "/etc/shadow"}, deny, "", "")
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
	if err := rwa.addPaths([]string{"/data/"}, allow, "", ""); err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	if rwa.files != nil {
		t.Errorf("addPaths files = %v, want nil", rwa.files)
	}
	if got := rwa.resolveAction("/data/x.txt", deny); got != allow {
		t.Errorf("resolveAction(/data/x.txt) = %s, want allow", got)
	}

	// No paths at all is not an error.
	if err := rwa.addPaths(nil, allow, "", ""); err != nil {
		t.Errorf("addPaths(nil) failed: %s", err)
	}
}

func TestAddPathsDuplicateFile(t *testing.T) {
	var rwa rwActions
	if err := rwa.addPaths([]string{"/a/b"}, allow, "", ""); err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	if err := rwa.addPaths([]string{"/a/b"}, deny, "", ""); err == nil {
		t.Error("addPaths with duplicate file path did not fail")
	}

	// Within a single call as well.
	var rwa2 rwActions
	if err := rwa2.addPaths([]string{"/a/b", "/a/b"}, allow, "", ""); err == nil {
		t.Error("addPaths with repeated file path did not fail")
	}
}

func TestNewRWActionsNil(t *testing.T) {
	rwa, err := newRWActions(nil, "", "")
	if err != nil {
		t.Fatalf("newRWActions(nil) failed: %s", err)
	}
	if rwa != nil {
		t.Fatalf("newRWActions(nil) = %v, want nil", rwa)
	}
	// A nil *rwActions always yields the default.
	for _, dflt := range []action{deny, ask, allow} {
		if got := rwa.resolveAction("/anything", dflt); got != dflt {
			t.Errorf("nil resolveAction(/anything, %s) = %s, want %s", dflt, got, dflt)
		}
	}
}

func TestNewRWActionsEmpty(t *testing.T) {
	rwa, err := newRWActions(&config.RWActions{}, "", "")
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	if rwa == nil {
		t.Fatal("newRWActions(&config.RWActions{}) = nil, want non-nil")
	}
	if got := rwa.resolveAction("/anything", ask); got != ask {
		t.Errorf("resolveAction with no paths = %s, want ask", got)
	}
}

// testPath returns a function which places a path beneath a temporary
// directory, leaving it otherwise untouched. Nothing beneath that directory
// exists unless the test creates it, so which paths are directories on disk is
// under the test's control.
func testPath(t *testing.T) func(path string) string {
	t.Helper()

	root := t.TempDir()
	return func(path string) string {
		return root + "/" + path
	}
}

func TestPathAction(t *testing.T) {
	// None of these paths exists, so every one without a trailing "/" is a file
	// rule.
	path := testPath(t)
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{path("home/"), path("etc/hosts")},
		Ask:   []string{path("home/secrets/"), path("tmp/scratch")},
		Deny:  []string{path("home/.ssh")},
	}, "", "")
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	cases := []struct {
		path string
		want action
	}{
		{path("etc/hosts"), allow},          // exact file allow
		{path("tmp/scratch"), ask},          // exact file ask
		{path("home/.ssh"), deny},           // exact file deny
		{path("home/notes.txt"), allow},     // beneath the allowed directory
		{path("home/sub/notes.txt"), allow}, // deeper beneath it
		{path("home/.ssh/config"), allow},   // a file rule does not cover children
		{path("etc/hostsX"), deny},          // no prefix matching on a file rule
		{path("var/log/messages"), deny},    // nothing matches -> default
		{path("home"), allow},               // the directory itself, so it can be made & removed
		{path("homex"), deny},               // but not a sibling sharing the prefix
		{path("home/secrets/keys"), ask},    // more specific ask beats the allow
		{path("home/secrets/a/b.txt"), ask}, // deeper beneath the ask directory
	}

	for _, c := range cases {
		if got := rwa.resolveAction(c.path, deny); got != c.want {
			t.Errorf("resolveAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestPathActionExistingDir(t *testing.T) {
	// A path which is a directory on disk is a directory rule even without a
	// trailing "/", so it covers everything within it; a path which is a file is
	// not.
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) failed: %s", sub, err)
	}
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %s", file, err)
	}

	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{root},
		Deny:  []string{sub, file},
	}, "", "")
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	wantDirs := []dirAction{{sub + "/", deny}, {root + "/", allow}}
	if !slices.Equal(rwa.dirs, wantDirs) {
		t.Errorf("dirs = %v, want %v", rwa.dirs, wantDirs)
	}
	if wantFiles := map[string]action{file: deny}; !maps.Equal(rwa.files, wantFiles) {
		t.Errorf("files = %v, want %v", rwa.files, wantFiles)
	}

	cases := []struct {
		path string
		want action
	}{
		{filepath.Join(sub, "x.txt"), deny},       // within the denied directory
		{filepath.Join(sub, "a", "b.txt"), deny},  // deeper within it
		{file, deny},                              // the denied file itself
		{filepath.Join(file, "child"), allow},     // a file rule does not cover children
		{filepath.Join(root, "other.txt"), allow}, // elsewhere beneath the allowed root
	}

	for _, c := range cases {
		if got := rwa.resolveAction(c.path, deny); got != c.want {
			t.Errorf("resolveAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestPathActionClean(t *testing.T) {
	// The configured paths are cleaned as they are expanded, so "." and ".."
	// elements in the configuration match the same rules as the cleaned paths
	// do. resolveAction itself is given already expanded paths; that unclean paths
	// can not slip past a rule is checked in TestCheckRWUncleanPath.
	path := testPath(t)
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{path("home/")},
		Deny:  []string{path("home/./secrets/keys/"), path("home/x/../.ssh")},
	}, "", "")
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	wantDirs := []dirAction{{path("home/secrets/keys/"), deny}, {path("home/"), allow}}
	if !slices.Equal(rwa.dirs, wantDirs) {
		t.Errorf("dirs = %v, want %v", rwa.dirs, wantDirs)
	}
	if wantFiles := map[string]action{path("home/.ssh"): deny}; !maps.Equal(rwa.files, wantFiles) {
		t.Errorf("files = %v, want %v", rwa.files, wantFiles)
	}

	cases := []struct {
		path string
		want action
	}{
		{path("home/notes.txt"), allow},
		{path("home/secrets/keys/id_rsa"), deny},
		{path("home/.ssh"), deny},
	}

	for _, c := range cases {
		if got := rwa.resolveAction(c.path, deny); got != c.want {
			t.Errorf("resolveAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestPathActionRootDir(t *testing.T) {
	// Cleaning "/" must not turn it into "//".
	rwa, err := newRWActions(&config.RWActions{Allow: []string{"/"}}, "", "")
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	if wantDirs := []dirAction{{"/", allow}}; !slices.Equal(rwa.dirs, wantDirs) {
		t.Fatalf("dirs = %v, want %v", rwa.dirs, wantDirs)
	}
	for _, p := range []string{"/anything", "/a/b/c"} {
		if got := rwa.resolveAction(p, deny); got != allow {
			t.Errorf("resolveAction(%q) = %s, want allow", p, got)
		}
	}
}

func TestPathActionFileBeatsDir(t *testing.T) {
	// A file entry is consulted before any directory entry, whatever the
	// directory's action.
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/data/"},
		Deny:  []string{"/data/secret.txt"},
	}, "", "")
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	if got := rwa.resolveAction("/data/secret.txt", allow); got != deny {
		t.Errorf("resolveAction(/data/secret.txt) = %s, want deny", got)
	}
	if got := rwa.resolveAction("/data/other.txt", deny); got != allow {
		t.Errorf("resolveAction(/data/other.txt) = %s, want allow", got)
	}
}

func TestPathActionDefault(t *testing.T) {
	rwa, err := newRWActions(&config.RWActions{Allow: []string{"/data/"}}, "", "")
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	// An unmatched path takes the default, whatever it is.
	for _, dflt := range []action{deny, ask, allow} {
		if got := rwa.resolveAction("/elsewhere/x", dflt); got != dflt {
			t.Errorf("resolveAction(/elsewhere/x, %s) = %s, want %s", dflt, got, dflt)
		}
	}
}

func TestNewRWActionsDuplicateFile(t *testing.T) {
	_, err := newRWActions(&config.RWActions{
		Allow: []string{"/a/b"},
		Deny:  []string{"/a/b"},
	}, "", "")
	if err == nil {
		t.Error("newRWActions with a duplicate file path did not fail")
	}
}

func TestNewRWActionsDuplicateDir(t *testing.T) {
	_, err := newRWActions(&config.RWActions{
		Allow: []string{"/a/b/"},
		Deny:  []string{"/a/b/"},
	}, "", "")
	if err == nil {
		t.Error("newRWActions with a duplicate directory did not fail")
	}

	// The duplicate is not adjacent in the order the paths are added, so
	// finding it depends on the directories being sorted first.
	_, err = newRWActions(&config.RWActions{
		Allow: []string{"/a/b/", "/c/d/"},
		Deny:  []string{"/a/b/"},
	}, "", "")
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
	}, "", "")
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
		if got := rwa.resolveAction(c.path, deny); got != c.want {
			t.Errorf("resolveAction(%q) = %s, want %s", c.path, got, c.want)
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

// checkRW expands and checks path, dropping the expanded path, for the tests
// which only care whether the operation is allowed. The paths they use are
// absolute, so expansion leaves them as they are.
func checkRW(sb *Sandbox, rwa *rwActions, op, path string) error {
	_, err := sb.expandCheckRW(rwa, op, path)
	return err
}

func TestCheckRW(t *testing.T) {
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read:  &config.RWActions{Allow: []string{"/data/", "/etc/hosts"}},
			Write: &config.RWActions{Allow: []string{"/data/out/"}},
		}, nil)

	// Reads are governed by the read actions.
	if err := checkRW(sb, sb.readActions, "read", "/data/in.txt"); err != nil {
		t.Errorf("checkRW(read, /data/in.txt) failed: %s", err)
	}
	if err := checkRW(sb, sb.readActions, "read", "/etc/hosts"); err != nil {
		t.Errorf("checkRW(read, /etc/hosts) failed: %s", err)
	}
	wantPathError(t, checkRW(sb, sb.readActions, "read", "/etc/shadow"), "read", "/etc/shadow")

	// Writes are governed by the write actions, which are separate: /data/ is
	// readable but only /data/out/ is writable.
	if err := checkRW(sb, sb.writeActions, "write", "/data/out/x.txt"); err != nil {
		t.Errorf("checkRW(write, /data/out/x.txt) failed: %s", err)
	}
	wantPathError(t, checkRW(sb, sb.writeActions, "write", "/data/in.txt"), "write", "/data/in.txt")
	wantPathError(t, checkRW(sb, sb.writeActions, "write", "/etc/hosts"), "write", "/etc/hosts")
}

func TestCheckRWAsk(t *testing.T) {
	cfg := &config.SandboxConfig{
		Read:  &config.RWActions{Ask: []string{"/data/"}},
		Write: &config.RWActions{Ask: []string{"/data/"}},
	}

	// Granted: no error, and the op and path are passed through to the ask func.
	ar := &askRecorder{resp: true}
	sb := checkSandbox(t, cfg, ar.ask)
	if err := checkRW(sb, sb.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW read with granted ask failed: %s", err)
	}
	if ar.calls != 1 {
		t.Errorf("ask func called %d times, want 1", ar.calls)
	}
	if ar.lastOp != "read" || ar.lastPath != "/data/x.txt" {
		t.Errorf("ask func(%q, %q), want (%q, %q)", ar.lastOp, ar.lastPath, "read",
			"/data/x.txt")
	}

	if err := checkRW(sb, sb.writeActions, "write", "/data/x.txt"); err != nil {
		t.Errorf("checkRW write with granted ask failed: %s", err)
	}
	if ar.lastOp != "write" {
		t.Errorf("ask func op = %q, want %q", ar.lastOp, "write")
	}

	// Declined: a permission error.
	sb = checkSandbox(t, cfg, (&askRecorder{resp: false}).ask)
	wantPathError(t, checkRW(sb, sb.readActions, "read", "/data/x.txt"), "read", "/data/x.txt")
	wantPathError(t, checkRW(sb, sb.writeActions, "write", "/data/x.txt"), "write", "/data/x.txt")
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

	if err := checkRW(sb, sb.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW(read, /data/x.txt) failed: %s", err)
	}
	wantPathError(t, checkRW(sb, sb.readActions, "read", "/data/no/x.txt"), "read",
		"/data/no/x.txt")
	wantPathError(t, checkRW(sb, sb.writeActions, "write", "/elsewhere"), "write", "/elsewhere")

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

	// The path is cleaned before matching, so ".." cannot escape a deny; the
	// error names the expanded path, which is the one that was checked.
	const unclean = "/data/yes/../no/x.txt"
	wantPathError(t, checkRW(sb, sb.readActions, "read", unclean), "read", "/data/no/x.txt")
	wantPathError(t, checkRW(sb, sb.writeActions, "write", unclean), "write", "/data/no/x.txt")

	// An unclean path that lands somewhere allowed is still allowed.
	if err := checkRW(sb, sb.readActions, "read", "/data/./a/../x.txt"); err != nil {
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
	if err := checkRW(sb, sb.readActions, "read", "/data/x.txt"); err != nil {
		t.Errorf("checkRW(read, /data/x.txt) failed: %s", err)
	}
	wantPathError(t, checkRW(sb, sb.writeActions, "write", "/data/x.txt"), "write", "/data/x.txt")

	// A config block with no paths in it denies everything too.
	sb = checkSandbox(t, &config.SandboxConfig{
		Read:  &config.RWActions{},
		Write: &config.RWActions{},
	}, nil)
	wantPathError(t, checkRW(sb, sb.readActions, "read", "/anything"), "read", "/anything")
	wantPathError(t, checkRW(sb, sb.writeActions, "write", "/anything"), "write", "/anything")
}

func TestCheckRWNoSandbox(t *testing.T) {
	// No sandbox configured at all: unrestricted access, and the user is never
	// prompted.
	ar := &askRecorder{resp: false}
	sb := checkSandbox(t, nil, ar.ask)

	for _, p := range []string{"/anything", "/etc/shadow", "./relative/path", "~/x"} {
		if err := checkRW(sb, sb.readActions, "read", p); err != nil {
			t.Errorf("checkRW(read, %q) failed: %s", p, err)
		}
		if err := checkRW(sb, sb.writeActions, "write", p); err != nil {
			t.Errorf("checkRW(write, %q) failed: %s", p, err)
		}
	}
	if ar.calls != 0 {
		t.Errorf("ask func called %d times, want 0", ar.calls)
	}

	// Expansion happens whether or not a sandbox is configured, so a path which
	// can not be expanded is refused even when nothing is restricted.
	if err := checkRW(sb, sb.readActions, "read", "relative/path"); err == nil {
		t.Errorf("checkRW(read, %q) did not fail", "relative/path")
	}
}

func TestExpandCheckRW(t *testing.T) {
	// The path being checked goes through the same expansion as the configured
	// paths, and the expanded path is what is returned.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := t.TempDir()
	t.Chdir(cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %s", err)
	}

	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read: &config.RWActions{
				Allow: []string{"./data/", "~/notes.txt"},
				Deny:  []string{"./data/secret/"},
			},
		}, nil)

	cases := []struct {
		path string
		want string
	}{
		{"./data/x.txt", filepath.Join(cwd, "data", "x.txt")},
		{"./data/a/../x.txt", filepath.Join(cwd, "data", "x.txt")},
		{"~/notes.txt", filepath.Join(home, "notes.txt")},
		// The same file named absolutely is the same file.
		{filepath.Join(cwd, "data", "x.txt"), filepath.Join(cwd, "data", "x.txt")},
	}

	for _, c := range cases {
		got, err := sb.expandCheckRW(sb.readActions, "read", c.path)
		if err != nil {
			t.Errorf("expandCheckRW(%q) failed: %s", c.path, err)
		} else if got != c.want {
			t.Errorf("expandCheckRW(%q) = %q, want %q", c.path, got, c.want)
		}
	}

	// cwd itself is outside of the allowed directory, and the error names the
	// expanded path.
	_, err = sb.expandCheckRW(sb.readActions, "read", ".")
	wantPathError(t, err, "read", cwd)

	// A rule written with "./" still denies what is beneath it, however the
	// path being checked is written.
	for _, p := range []string{"./data/secret/keys", filepath.Join(cwd, "data/secret/keys")} {
		_, err := sb.expandCheckRW(sb.readActions, "read", p)
		wantPathError(t, err, "read", filepath.Join(cwd, "data", "secret", "keys"))
	}

	// A path which can not be expanded fails before any action is consulted,
	// with the expansion error rather than a permission error.
	_, err = sb.expandCheckRW(sb.readActions, "read", "data/x.txt")
	if err == nil {
		t.Errorf("expandCheckRW of a bare relative path did not fail")
	} else if errors.Is(err, fs.ErrPermission) {
		t.Errorf("expandCheckRW of a bare relative path = %v, want an expansion error", err)
	}
}

func TestExpandCheckRWAsk(t *testing.T) {
	// The user is asked about the expanded path, not the one as written.
	cwd := t.TempDir()
	t.Chdir(cwd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %s", err)
	}

	ar := &askRecorder{resp: true}
	sb := checkSandbox(t,
		&config.SandboxConfig{Read: &config.RWActions{Ask: []string{"./data/"}}}, ar.ask)

	got, err := sb.expandCheckRW(sb.readActions, "read", "./data/x.txt")
	if err != nil {
		t.Fatalf("expandCheckRW failed: %s", err)
	}
	want := filepath.Join(cwd, "data", "x.txt")
	if got != want {
		t.Errorf("expandCheckRW = %q, want %q", got, want)
	}
	if ar.calls != 1 || ar.lastOp != "read" || ar.lastPath != want {
		t.Errorf("ask called %d times with (%q, %q), want 1 time with (%q, %q)", ar.calls,
			ar.lastOp, ar.lastPath, "read", want)
	}
}

func TestSandboxOpsExpand(t *testing.T) {
	// The operations act on the expanded path, so a relative or ~ path reaches
	// the file the rules were checked against.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := t.TempDir()
	t.Chdir(cwd)

	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read:  &config.RWActions{Allow: []string{"./", "~/"}},
			Write: &config.RWActions{Allow: []string{"./", "~/"}},
		}, nil)

	if err := sb.WriteFile("./x.txt", []byte("cwd"), 0o644); err != nil {
		t.Fatalf("WriteFile(./x.txt) failed: %s", err)
	}
	if err := sb.WriteFile("~/x.txt", []byte("home"), 0o644); err != nil {
		t.Fatalf("WriteFile(~/x.txt) failed: %s", err)
	}

	// Each landed where it was expanded to, and not in the other directory.
	for _, c := range []struct{ dir, want string }{{cwd, "cwd"}, {home, "home"}} {
		data, err := os.ReadFile(filepath.Join(c.dir, "x.txt"))
		if err != nil {
			t.Fatalf("ReadFile(%q) failed: %s", filepath.Join(c.dir, "x.txt"), err)
		}
		if string(data) != c.want {
			t.Errorf("%s/x.txt = %q, want %q", c.dir, data, c.want)
		}
	}

	for _, c := range []struct{ path, want string }{{"./x.txt", "cwd"}, {"~/x.txt", "home"}} {
		data, err := sb.ReadFile(c.path)
		if err != nil {
			t.Errorf("ReadFile(%q) failed: %s", c.path, err)
		} else if string(data) != c.want {
			t.Errorf("ReadFile(%q) = %q, want %q", c.path, data, c.want)
		}
	}

	// MkdirAll, Stat, Remove and WalkDir expand too.
	if err := sb.MkdirAll("./sub/deep", 0o755); err != nil {
		t.Errorf("MkdirAll(./sub/deep) failed: %s", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "sub", "deep")); err != nil {
		t.Errorf("MkdirAll(./sub/deep) did not create %s/sub/deep: %s", cwd, err)
	}
	if fi, err := sb.Stat("./sub"); err != nil {
		t.Errorf("Stat(./sub) failed: %s", err)
	} else if !fi.IsDir() {
		t.Errorf("Stat(./sub) is not a directory")
	}
	if err := sb.Remove("./sub/deep"); err != nil {
		t.Errorf("Remove(./sub/deep) failed: %s", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "sub", "deep")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove(./sub/deep) did not remove %s/sub/deep", cwd)
	}

	var walked []string
	err := sb.WalkDir("./sub", func(path string, d fs.DirEntry, err error) error {
		walked = append(walked, path)
		return err
	})
	if err != nil {
		t.Errorf("WalkDir(./sub) failed: %s", err)
	}
	if want := []string{filepath.Join(cwd, "sub")}; !slices.Equal(walked, want) {
		t.Errorf("WalkDir(./sub) walked %v, want %v", walked, want)
	}

	// A path which can not be expanded fails, without touching the file system.
	if err := sb.WriteFile("x.txt", []byte("bare"), 0o644); err == nil {
		t.Errorf("WriteFile of a bare relative path did not fail")
	}
	if _, err := os.Stat(filepath.Join(cwd, "x.txt")); err != nil {
		t.Errorf("the failed WriteFile disturbed %s/x.txt: %s", cwd, err)
	}
	if _, err := sb.ReadFile("x.txt"); err == nil {
		t.Errorf("ReadFile of a bare relative path did not fail")
	}
	if err := sb.MkdirAll("sub2", 0o755); err == nil {
		t.Errorf("MkdirAll of a bare relative path did not fail")
	}
	if _, err := sb.Stat("x.txt"); err == nil {
		t.Errorf("Stat of a bare relative path did not fail")
	}
	if err := sb.Remove("x.txt"); err == nil {
		t.Errorf("Remove of a bare relative path did not fail")
	}
	if err := sb.WalkDir("sub", func(path string, d fs.DirEntry, err error) error {
		t.Errorf("WalkDir of a bare relative path called the walk func for %q", path)
		return err
	}); err == nil {
		t.Errorf("WalkDir of a bare relative path did not fail")
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

func TestMkdirAllParents(t *testing.T) {
	// The parents which have to be created are checked as well, so that making
	// an allowed directory does not create a denied one on the way to it.
	root := t.TempDir()
	denied := filepath.Join(root, "denied")
	allowed := filepath.Join(denied, "sub", "allowed")

	sb := checkSandbox(t,
		&config.SandboxConfig{
			Write: &config.RWActions{
				Allow: []string{allowed + "/"},
				Deny:  []string{denied + "/"},
			},
		}, nil)

	wantPathError(t, sb.MkdirAll(allowed, 0o755), "mkdir", denied)
	if _, err := os.Stat(denied); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(%q) = %v, want the denied parent not to have been created", denied, err)
	}

	// Parents which already exist are not checked, because they will not be
	// created; only what is missing has to be allowed.
	nested := filepath.Join(root, "a", "b", "c")
	sb = checkSandbox(t,
		&config.SandboxConfig{
			Write: &config.RWActions{Allow: []string{root + "/"}},
		}, nil)
	if err := sb.MkdirAll(nested, 0o755); err != nil {
		t.Errorf("MkdirAll(%q) failed: %s", nested, err)
	}

	// An existing directory is made without checking any parent of it.
	sb = checkSandbox(t,
		&config.SandboxConfig{
			Write: &config.RWActions{
				Allow: []string{nested},
				Deny:  []string{root + "/"},
			},
		}, nil)
	if err := sb.MkdirAll(nested, 0o755); err != nil {
		t.Errorf("MkdirAll(%q) of an existing directory failed: %s", nested, err)
	}
}

func TestMkdirAllParentsAsk(t *testing.T) {
	// A missing parent is asked about in its own right, and nothing is created
	// when the asking is declined.
	root := t.TempDir()
	parent := filepath.Join(root, "a")
	path := filepath.Join(parent, "b")

	cfg := &config.SandboxConfig{
		Write: &config.RWActions{
			Allow: []string{path + "/"},
			Ask:   []string{root + "/"},
		},
	}

	ar := &askRecorder{resp: false}
	sb := checkSandbox(t, cfg, ar.ask)
	wantPathError(t, sb.MkdirAll(path, 0o755), "mkdir", parent)
	if ar.calls != 1 {
		t.Errorf("ask func called %d times, want 1", ar.calls)
	}
	if ar.lastOp != "mkdir" || ar.lastPath != parent {
		t.Errorf("ask func(%q, %q), want (%q, %q)", ar.lastOp, ar.lastPath, "mkdir", parent)
	}
	if _, err := os.Stat(parent); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(%q) = %v, want nothing to have been created", parent, err)
	}

	// Granted: the parent is created along with the path itself.
	ar = &askRecorder{resp: true}
	sb = checkSandbox(t, cfg, ar.ask)
	if err := sb.MkdirAll(path, 0o755); err != nil {
		t.Errorf("MkdirAll(%q) with granted ask failed: %s", path, err)
	}
	if ar.calls != 1 {
		t.Errorf("ask func called %d times, want 1", ar.calls)
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		t.Errorf("Stat(%q) = %v, want a directory", path, err)
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

// makeExec creates an executable in dir which echoes its own name and the
// arguments it was passed.
func makeExec(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho "+name+" \"$@\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) failed: %s", path, err)
	}
	return path
}

// execRules renders the rules for cmd, which must already be expanded, as
// "action:pattern,pattern" strings, in the order they will be matched.
func execRules(ea *execActions, cmd string) []string {
	var rules []string
	for _, ca := range ea.cmds[cmd] {
		var args []string
		for _, pat := range ca.pats {
			args = append(args, pat.String())
		}
		rules = append(rules, ca.act.String()+":"+strings.Join(args, ","))
	}
	return rules
}

func TestAddCmds(t *testing.T) {
	var ea execActions

	err := ea.addCmds([][]string{{"/cmds/git", "^status$"}, {"/usr/bin/"}}, allow, "", "")
	if err != nil {
		t.Fatalf("addCmds failed: %s", err)
	}
	err = ea.addCmds([][]string{{"/cmds/git"}, {"/usr/bin/priv/"}}, deny, "", "")
	if err != nil {
		t.Fatalf("addCmds failed: %s", err)
	}

	// The configured patterns are anchored, so that each one must match a whole
	// argument. The commands are paths which do not exist, so that expanding
	// them does not depend on what is installed or on how /bin is linked.
	wantCmds := []string{`allow:\A(?:^status$)\z`, "deny:"}
	if got := execRules(&ea, "/cmds/git"); !slices.Equal(got, wantCmds) {
		t.Errorf("addCmds rules for /cmds/git = %v, want %v", got, wantCmds)
	}

	wantDirs := []dirAction{{"/usr/bin/", allow}, {"/usr/bin/priv/", deny}}
	if !slices.Equal(ea.dirs, wantDirs) {
		t.Errorf("addCmds dirs = %v, want %v", ea.dirs, wantDirs)
	}
}

func TestNewExecActionsNil(t *testing.T) {
	ea, err := newExecActions(nil, "", "")
	if err != nil {
		t.Fatalf("newExecActions(nil) failed: %s", err)
	}
	if ea != nil {
		t.Fatalf("newExecActions(nil) = %v, want nil", ea)
	}
	// A nil *execActions always yields the default.
	for _, dflt := range []action{deny, ask, allow} {
		if got := ea.resolveAction("/cmds/git", []string{"status"}, dflt); got != dflt {
			t.Errorf("nil resolveAction(/cmds/git, %s) = %s, want %s", dflt, got, dflt)
		}
	}
}

func TestNewExecActionsEmpty(t *testing.T) {
	ea, err := newExecActions(&config.ExecuteActions{}, "", "")
	if err != nil {
		t.Fatalf("newExecActions failed: %s", err)
	}
	if ea == nil {
		t.Fatal("newExecActions(&config.ExecuteActions{}) = nil, want non-nil")
	}
	if got := ea.resolveAction("/cmds/git", nil, ask); got != ask {
		t.Errorf("resolveAction with no commands = %s, want ask", got)
	}
}

func TestNewExecActionsErrors(t *testing.T) {
	cases := []struct {
		what string
		cfg  *config.ExecuteActions
	}{
		{"an empty command", &config.ExecuteActions{Allow: [][]string{{}}}},
		{"arguments on a directory",
			&config.ExecuteActions{Allow: [][]string{{"/usr/bin/", "^status$"}}}},
		{"an invalid pattern", &config.ExecuteActions{Allow: [][]string{{"/cmds/git", "("}}}},
		{"a duplicate command",
			&config.ExecuteActions{
				Allow: [][]string{{"/cmds/git", "^status$"}},
				Deny:  [][]string{{"/cmds/git", "^status$"}},
			}},
		{"a duplicate command with no patterns",
			&config.ExecuteActions{Allow: [][]string{{"/cmds/git"}, {"/cmds/git"}}}},
		{"a duplicate directory",
			&config.ExecuteActions{
				Allow: [][]string{{"/usr/bin/"}},
				Deny:  [][]string{{"/usr/bin/"}},
			}},
		{"a non-adjacent duplicate directory",
			&config.ExecuteActions{
				Allow: [][]string{{"/usr/bin/"}, {"/opt/cmds/"}},
				Deny:  [][]string{{"/usr/bin/"}},
			}},
	}

	for _, c := range cases {
		if _, err := newExecActions(c.cfg, "", ""); err == nil {
			t.Errorf("newExecActions with %s did not fail", c.what)
		}
	}

	// The same command with different patterns is not a duplicate.
	_, err := newExecActions(&config.ExecuteActions{
		Allow: [][]string{{"/cmds/git", "^status$"}},
		Deny:  [][]string{{"/cmds/git", "^push$"}, {"/cmds/git"}},
	}, "", "")
	if err != nil {
		t.Errorf("newExecActions with distinct patterns failed: %s", err)
	}
}

func TestCmdActionPatterns(t *testing.T) {
	// The patterns constrain the leading arguments, one pattern per argument,
	// and the most specific matching rule applies.
	ea, err := newExecActions(&config.ExecuteActions{
		Allow: [][]string{{"/cmds/git", "^status$"}, {"/cmds/git", "^log$"}},
		Ask:   [][]string{{"/cmds/git", "^push$", "^origin$"}},
		Deny:  [][]string{{"/cmds/git"}},
	}, "", "")
	if err != nil {
		t.Fatalf("newExecActions failed: %s", err)
	}

	cases := []struct {
		args []string
		want action
	}{
		{[]string{"status"}, allow},
		{[]string{"status", "--short"}, allow}, // trailing arguments are unconstrained
		{[]string{"log"}, allow},
		{[]string{"push", "origin", "main"}, ask},
		{[]string{"push", "elsewhere"}, deny}, // the second pattern does not match
		{[]string{"push"}, deny},              // too few arguments for the ask rule
		{[]string{"commit"}, deny},
		{nil, deny},
	}

	for _, c := range cases {
		if got := ea.resolveAction("/cmds/git", c.args, allow); got != c.want {
			t.Errorf("resolveAction(/cmds/git, %v) = %s, want %s", c.args, got, c.want)
		}
	}
}

func TestCmdActionEquallySpecific(t *testing.T) {
	// Two rules constraining the same number of arguments both match; the more
	// restrictive one applies. A pattern matches a whole argument, so matching
	// every flag takes ".*" rather than a bare prefix.
	ea, err := newExecActions(&config.ExecuteActions{
		Allow: [][]string{{"/cmds/rm", "-.*"}},
		Deny:  [][]string{{"/cmds/rm", "^-rf$"}},
	}, "", "")
	if err != nil {
		t.Fatalf("newExecActions failed: %s", err)
	}

	if got := ea.resolveAction("/cmds/rm", []string{"-rf"}, allow); got != deny {
		t.Errorf("resolveAction(/cmds/rm, -rf) = %s, want deny", got)
	}
	if got := ea.resolveAction("/cmds/rm", []string{"-i"}, deny); got != allow {
		t.Errorf("resolveAction(/cmds/rm, -i) = %s, want allow", got)
	}
}

func TestCmdActionPatternsAnchored(t *testing.T) {
	// A pattern must match the whole argument. Were it unanchored, any argument
	// merely containing a match would satisfy the rule, letting an allow rule
	// pass the arguments it was written to exclude.
	ea, err := newExecActions(&config.ExecuteActions{
		Allow: [][]string{{"/cmds/sh", "-c", "echo"}},
	}, "", "")
	if err != nil {
		t.Fatalf("newExecActions failed: %s", err)
	}

	cases := []struct {
		args []string
		want action
	}{
		{[]string{"-c", "echo"}, allow},
		// Each of these merely contains an argument which is allowed.
		{[]string{"-c", "rm -rf /tmp/x # echo"}, deny},
		{[]string{"-c", "echo; rm -rf /tmp/x"}, deny},
		{[]string{"-c", "echoes"}, deny},
		{[]string{"-cx", "echo"}, deny},
	}

	for _, c := range cases {
		if got := ea.resolveAction("/cmds/sh", c.args, deny); got != c.want {
			t.Errorf("resolveAction(/cmds/sh, %q) = %s, want %s", c.args, got, c.want)
		}
	}

	// Anchoring is added to the pattern, not required of it, so a pattern which
	// anchors itself keeps working.
	ea, err = newExecActions(&config.ExecuteActions{
		Allow: [][]string{{"/cmds/sh", "^-c$"}},
	}, "", "")
	if err != nil {
		t.Fatalf("newExecActions failed: %s", err)
	}
	if got := ea.resolveAction("/cmds/sh", []string{"-c"}, deny); got != allow {
		t.Errorf("resolveAction(/cmds/sh, -c) = %s, want allow", got)
	}
}

func TestCmdActionDir(t *testing.T) {
	ea, err := newExecActions(&config.ExecuteActions{
		Allow: [][]string{{"/usr/bin/"}},
		Ask:   [][]string{{"/usr/bin/priv/"}},
		Deny:  [][]string{{"/usr/bin/dd"}},
	}, "", "")
	if err != nil {
		t.Fatalf("newExecActions failed: %s", err)
	}

	cases := []struct {
		cmd  string
		want action
	}{
		{"/usr/bin/ls", allow},     // within the allowed directory
		{"/usr/bin/dd", deny},      // a command rule beats the directory
		{"/usr/bin/priv/x", ask},   // the most specific directory
		{"/usr/bin/priv/a/b", ask}, // deeper beneath it
		{"/cmds/ls", deny},         // nothing matches -> default
		{"/usr/binary/ls", deny},   // no prefix matching short of a path element
	}

	for _, c := range cases {
		if got := ea.resolveAction(c.cmd, nil, deny); got != c.want {
			t.Errorf("resolveAction(%q) = %s, want %s", c.cmd, got, c.want)
		}
	}
}

func TestCmdActionUnmatchedPatterns(t *testing.T) {
	// A rule for a command which does not match its arguments is not a rule for
	// that command at all, so the directory it is in still applies.
	ea, err := newExecActions(&config.ExecuteActions{
		Allow: [][]string{{"/usr/bin/"}},
		Deny:  [][]string{{"/usr/bin/git", "^push$"}},
	}, "", "")
	if err != nil {
		t.Fatalf("newExecActions failed: %s", err)
	}

	if got := ea.resolveAction("/usr/bin/git", []string{"status"}, deny); got != allow {
		t.Errorf("resolveAction(/usr/bin/git, status) = %s, want allow", got)
	}
	if got := ea.resolveAction("/usr/bin/git", []string{"push"}, allow); got != deny {
		t.Errorf("resolveAction(/usr/bin/git, push) = %s, want deny", got)
	}
}

func TestCmdActionLookPath(t *testing.T) {
	dir := t.TempDir()
	path := makeExec(t, dir, "prog")
	t.Setenv("PATH", dir)

	// A command configured by name is resolved through PATH, so a rule for the
	// name is a rule for the path it resolves to.
	ea, err := newExecActions(&config.ExecuteActions{Allow: [][]string{{"prog"}}}, dir, dir)
	if err != nil {
		t.Fatalf("newExecActions failed: %s", err)
	}

	if got := ea.resolveAction(path, nil, deny); got != allow {
		t.Errorf("resolveAction(%q) = %s, want allow", path, got)
	}

	// Another executable of the same name elsewhere is not the one allowed.
	other := makeExec(t, t.TempDir(), "prog")
	if got := ea.resolveAction(other, nil, deny); got != deny {
		t.Errorf("resolveAction(%q) = %s, want deny", other, got)
	}

	// A command which can not be expanded can not be configured at all.
	for _, cmd := range []string{"no-such-command", "relative/prog"} {
		cfg := &config.ExecuteActions{Allow: [][]string{{cmd}}}
		if _, err := newExecActions(cfg, dir, dir); err == nil {
			t.Errorf("newExecActions with %q did not fail", cmd)
		}
	}
}

// checkExec expands and checks a command, dropping the expanded path, for the
// tests which only care whether the command may run. The commands they use are
// absolute paths, so expansion leaves them as they are.
func checkExec(sb *Sandbox, name string, args []string) error {
	_, err := sb.expandCheckExec(name, args)
	return err
}

func TestSymlinkResolved(t *testing.T) {
	// A link is resolved before it is checked, so that a link within an allowed
	// directory cannot reach a denied file outside of it, and a link to a denied
	// command cannot run it.
	root := t.TempDir()
	inside, outside := filepath.Join(root, "inside"), filepath.Join(root, "outside")
	for _, dir := range []string{inside, outside} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("Mkdir(%q) failed: %s", dir, err)
		}
	}

	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) failed: %s", secret, err)
	}
	escape := filepath.Join(inside, "escape.txt")
	if err := os.Symlink(secret, escape); err != nil {
		t.Fatalf("Symlink(%q) failed: %s", escape, err)
	}

	plain := filepath.Join(inside, "plain.txt")
	if err := os.WriteFile(plain, []byte("plain"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) failed: %s", plain, err)
	}
	within := filepath.Join(inside, "within.txt")
	if err := os.Symlink(plain, within); err != nil {
		t.Fatalf("Symlink(%q) failed: %s", within, err)
	}

	sb := checkSandbox(t, &config.SandboxConfig{
		Read: &config.RWActions{
			Allow: []string{inside + "/"},
			Deny:  []string{outside + "/"},
		},
		Write: &config.RWActions{
			Allow: []string{inside + "/"},
			Deny:  []string{outside + "/"},
		},
	}, nil)

	if _, err := sb.ReadFile(escape); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("ReadFile through a link to a denied file = %v, want permission error", err)
	}
	if err := sb.WriteFile(escape, []byte("x"), 0o600); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("WriteFile through a link to a denied file = %v, want permission error", err)
	}
	if b, err := os.ReadFile(secret); err != nil || string(b) != "secret" {
		t.Errorf("denied file = %q, %v; want it left as it was", b, err)
	}

	// A link to a file which is allowed still reads, as its target.
	if b, err := sb.ReadFile(within); err != nil || string(b) != "plain" {
		t.Errorf("ReadFile(%q) = %q, %v; want %q", within, b, err, "plain")
	}

	// The same for a command: the denied command cannot be run through a link
	// to it from an allowed directory.
	cmd := makeExec(t, outside, "gaitcmd")
	alias := filepath.Join(inside, "alias")
	if err := os.Symlink(cmd, alias); err != nil {
		t.Fatalf("Symlink(%q) failed: %s", alias, err)
	}

	sb = checkSandbox(t, &config.SandboxConfig{
		Execute: &config.ExecuteActions{
			Allow: [][]string{{inside + "/"}},
			Deny:  [][]string{{cmd}},
		},
	}, nil)
	wantPathError(t, checkExec(sb, alias, nil), "execute", cmd)
}

func TestExpandCheckExecLookPath(t *testing.T) {
	// A command run by name is resolved through PATH, so it is checked, and run,
	// as the path it resolves to.
	dir := t.TempDir()
	path := makeExec(t, dir, "prog")
	t.Setenv("PATH", dir)

	ar := &askRecorder{resp: true}
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Execute: &config.ExecuteActions{Ask: [][]string{{path}}},
		}, ar.ask)

	got, err := sb.expandCheckExec("prog", []string{"hello"})
	if err != nil {
		t.Errorf("expandCheckExec(prog) failed: %s", err)
	}
	if got != path {
		t.Errorf("expandCheckExec(prog) = %q, want %q", got, path)
	}
	if want := path + " hello"; ar.lastPath != want {
		t.Errorf("ask func path = %q, want %q", ar.lastPath, want)
	}

	// A relative command is rejected, just as it is for reads and writes.
	if err := checkExec(sb, "relative/prog", nil); err == nil {
		t.Error("expandCheckExec of a relative command did not fail")
	}
}

func TestCheckExec(t *testing.T) {
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Execute: &config.ExecuteActions{
				Allow: [][]string{{"/cmds/echo"}},
				Deny:  [][]string{{"/cmds/echo", "^secret$"}},
			},
		}, nil)

	if err := checkExec(sb, "/cmds/echo", []string{"hello"}); err != nil {
		t.Errorf("checkExec(/cmds/echo, hello) failed: %s", err)
	}
	wantPathError(t, checkExec(sb, "/cmds/echo", []string{"secret"}), "execute", "/cmds/echo secret")
	wantPathError(t, checkExec(sb, "/cmds/false", nil), "execute", "/cmds/false")
}

func TestCheckExecAsk(t *testing.T) {
	cfg := &config.SandboxConfig{
		Execute: &config.ExecuteActions{
			Allow: [][]string{{"/cmds/echo"}},
			Ask:   [][]string{{"/cmds/git", "^push$"}},
		},
	}

	// Granted: no error, and the whole command line is passed to the ask func.
	ar := &askRecorder{resp: true}
	sb := checkSandbox(t, cfg, ar.ask)
	if err := checkExec(sb, "/cmds/git", []string{"push", "origin"}); err != nil {
		t.Errorf("checkExec with granted ask failed: %s", err)
	}
	if ar.calls != 1 {
		t.Errorf("ask func called %d times, want 1", ar.calls)
	}
	if ar.lastOp != "execute" || ar.lastPath != "/cmds/git push origin" {
		t.Errorf("ask func(%q, %q), want (%q, %q)", ar.lastOp, ar.lastPath, "execute",
			"/cmds/git push origin")
	}

	// The ask func is only consulted for an ask action, never for allow or for a
	// denied command.
	if err := checkExec(sb, "/cmds/echo", []string{"hello"}); err != nil {
		t.Errorf("checkExec(/cmds/echo, hello) failed: %s", err)
	}
	wantPathError(t, checkExec(sb, "/cmds/git", []string{"status"}), "execute", "/cmds/git status")
	if ar.calls != 1 {
		t.Errorf("ask func called %d times, want 1", ar.calls)
	}

	// Declined: a permission error.
	sb = checkSandbox(t, cfg, (&askRecorder{resp: false}).ask)
	wantPathError(t, checkExec(sb, "/cmds/git", []string{"push"}), "execute", "/cmds/git push")

	// A nil ask func denies rather than panicking on a nil call.
	sb = checkSandbox(t, cfg, nil)
	wantPathError(t, checkExec(sb, "/cmds/git", []string{"push"}), "execute", "/cmds/git push")
}

func TestQuoteCmdline(t *testing.T) {
	// Ordinary arguments are left as they are, so that the command line stays
	// readable; anything which would make it ambiguous is quoted.
	cases := []struct {
		path string
		args []string
		want string
	}{
		{"/cmds/git", nil, "/cmds/git"},
		{"/cmds/git", []string{"push", "origin"}, "/cmds/git push origin"},
		// One argument with spaces is not several arguments.
		{"/cmds/sh", []string{"-c", "rm -rf /"}, `/cmds/sh -c "rm -rf /"`},
		// An empty argument is visible rather than being lost.
		{"/cmds/sh", []string{"-c", ""}, `/cmds/sh -c ""`},
		// Control characters cannot forge further lines in the prompt.
		{"/cmds/sh", []string{"-c", "a\nallow: yes"}, `/cmds/sh -c "a\nallow: yes"`},
		{"/cmds/sh", []string{"-c", "a\tb"}, `/cmds/sh -c "a\tb"`},
		// Quotes and backslashes are escaped, so what is quoted is unambiguous.
		{"/cmds/sh", []string{`a"b`}, `/cmds/sh "a\"b"`},
		{"/cmds/sh", []string{`a\b`}, `/cmds/sh "a\\b"`},
		// Escapes and bytes which are not valid UTF-8 do not reach the terminal
		// as they are.
		{"/cmds/sh", []string{"\x1b[2K"}, `/cmds/sh "\x1b[2K"`},
		{"/cmds/sh", []string{"a\xffb"}, `/cmds/sh "a\xffb"`},
		// A path with a space in it is quoted too.
		{"/cmds dir/git", []string{"push"}, `"/cmds dir/git" push`},
	}

	for _, c := range cases {
		if got := quoteCmdline(c.path, c.args); got != c.want {
			t.Errorf("quoteCmdline(%q, %q) = %s, want %s", c.path, c.args, got, c.want)
		}
	}
}

func TestCheckExecAskQuoted(t *testing.T) {
	// What the user is asked about, and what the permission error reports, is
	// the quoted command line.
	cfg := &config.SandboxConfig{
		Execute: &config.ExecuteActions{
			Ask:  [][]string{{"/cmds/sh", "-c"}},
			Deny: [][]string{{"/cmds/sh", "-c", "rm.*"}},
		},
	}

	ar := &askRecorder{resp: true}
	sb := checkSandbox(t, cfg, ar.ask)
	if err := checkExec(sb, "/cmds/sh", []string{"-c", "echo hello"}); err != nil {
		t.Errorf("checkExec with granted ask failed: %s", err)
	}
	if want := `/cmds/sh -c "echo hello"`; ar.lastPath != want {
		t.Errorf("ask func path = %q, want %q", ar.lastPath, want)
	}

	wantPathError(t, checkExec(sb, "/cmds/sh", []string{"-c", "rm -rf /"}), "execute",
		`/cmds/sh -c "rm -rf /"`)
}

func TestCheckExecMissingConfig(t *testing.T) {
	// Within a configured sandbox, an absent execute block denies every command.
	sb := checkSandbox(t, &config.SandboxConfig{
		Read: &config.RWActions{Allow: []string{"/data/"}},
	}, nil)
	if sb.executeActions != nil {
		t.Fatalf("executeActions = %v, want nil", sb.executeActions)
	}
	wantPathError(t, checkExec(sb, "/cmds/echo", nil), "execute", "/cmds/echo")

	// An execute block with no commands in it denies everything too.
	sb = checkSandbox(t, &config.SandboxConfig{Execute: &config.ExecuteActions{}}, nil)
	wantPathError(t, checkExec(sb, "/cmds/echo", nil), "execute", "/cmds/echo")
}

func TestCheckExecNoSandbox(t *testing.T) {
	// No sandbox configured at all: any command which expands may run, and the
	// user is never prompted.
	ar := &askRecorder{resp: false}
	sb := checkSandbox(t, nil, ar.ask)

	for _, cmd := range []string{"/cmds/echo", "sh"} {
		if err := checkExec(sb, cmd, []string{"-rf", "/"}); err != nil {
			t.Errorf("checkExec(%q) failed: %s", cmd, err)
		}
	}

	// The command still has to expand: a relative one does not.
	if err := checkExec(sb, "relative/prog", nil); err == nil {
		t.Error("checkExec of a relative command did not fail")
	}
	if ar.calls != 0 {
		t.Errorf("ask func called %d times, want 0", ar.calls)
	}
}

func TestCombinedOutput(t *testing.T) {
	dir := t.TempDir()
	allowed := makeExec(t, dir, "allowed")
	denied := makeExec(t, dir, "denied")

	sb := checkSandbox(t,
		&config.SandboxConfig{
			Execute: &config.ExecuteActions{Allow: [][]string{{allowed}}},
		}, nil)

	out, err := sb.CombinedOutput(context.Background(), dir, allowed, "hello")
	if err != nil {
		t.Errorf("CombinedOutput(%q) failed: %s", allowed, err)
	}
	if got := strings.TrimSpace(string(out)); got != "allowed hello" {
		t.Errorf("CombinedOutput(%q) = %q, want %q", allowed, got, "allowed hello")
	}

	// A denied command is not run at all.
	out, err = sb.CombinedOutput(context.Background(), dir, denied)
	wantPathError(t, err, "execute", denied)
	if out != nil {
		t.Errorf("CombinedOutput(%q) = %q, want no output", denied, out)
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
