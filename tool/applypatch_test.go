package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func wrapPatch(body string) string {
	return "*** Begin Patch\n" + body + "\n*** End Patch"
}

func runApplyPatch(t *testing.T, patch string) (string, error) {
	t.Helper()

	buf, err := json.Marshal(applyPatchArgs{Input: patch})
	if err != nil {
		t.Fatal(err)
	}
	return applyPatch(context.Background(), testSandbox(), buf)
}

func testParsePatch(t *testing.T, patch string, hunks []patchHunk, errMsg string) {
	t.Helper()

	ret, err := parsePatch(patch)
	if errMsg != "" {
		if err == nil {
			t.Errorf("parsePatch(%q) did not fail; expected %q", patch, errMsg)
		} else if err.Error() != errMsg {
			t.Errorf("parsePatch(%q) failed with %q; expected %q", patch, err, errMsg)
		}
	} else if err != nil {
		t.Errorf("parsePatch(%q) failed with %s", patch, err)
	} else if !reflect.DeepEqual(ret, hunks) {
		t.Errorf("parsePatch(%q) got %#v; expected %#v", patch, ret, hunks)
	}
}

func TestParsePatch(t *testing.T) {
	testParsePatch(t, "bad", nil,
		"Invalid patch: The first line of the patch must be '*** Begin Patch'")
	testParsePatch(t, "*** Begin Patch\nbad", nil,
		"Invalid patch: The last line of the patch must be '*** End Patch'")
	testParsePatch(t,
		"*** Begin Patch\n*** Update File: test.py\n*** End Patch", nil,
		"Invalid patch hunk on line 2: Update file hunk for path 'test.py' is empty")
	testParsePatch(t, "*** Begin Patch\n*** End Patch", nil, "")
	testParsePatch(t,
		"*** Begin Patch\n"+
			"*** Add File: path/add.py\n"+
			"+abc\n"+
			"+def\n"+
			"*** Delete File: path/delete.py\n"+
			"*** Update File: path/update.py\n"+
			"*** Move to: path/update2.py\n"+
			"@@ def f():\n"+
			"-    pass\n"+
			"+    return 123\n"+
			"*** End Patch",
		[]patchHunk{
			{op: addFileOp, path: "path/add.py", contents: "abc\ndef\n"},
			{op: deleteFileOp, path: "path/delete.py"},
			{
				op:       updateFileOp,
				path:     "path/update.py",
				movePath: "path/update2.py",
				chunks: []updateChunk{
					{
						changeContext:    "def f():",
						hasChangeContext: true,
						oldLines:         []string{"    pass"},
						newLines:         []string{"    return 123"},
					},
				},
			},
		}, "")

	// Update hunk followed by another hunk (Add File).
	testParsePatch(t,
		"*** Begin Patch\n"+
			"*** Update File: file.py\n"+
			"@@\n"+
			"+line\n"+
			"*** Add File: other.py\n"+
			"+content\n"+
			"*** End Patch",
		[]patchHunk{
			{
				op:   updateFileOp,
				path: "file.py",
				chunks: []updateChunk{
					{newLines: []string{"line"}},
				},
			},
			{op: addFileOp, path: "other.py", contents: "content\n"},
		}, "")

	// Update hunk without an explicit @@ header for the first chunk should parse.
	testParsePatch(t,
		"*** Begin Patch\n"+
			"*** Update File: file2.py\n"+
			" import foo\n"+
			"+bar\n"+
			"*** End Patch",
		[]patchHunk{
			{
				op:   updateFileOp,
				path: "file2.py",
				chunks: []updateChunk{
					{
						oldLines: []string{"import foo"},
						newLines: []string{"import foo", "bar"},
					},
				},
			},
		}, "")

	testParsePatch(t,
		"*** Begin Patch\n"+
			"*** Update File: file.py\n"+
			"@@\n"+
			"bad\n"+
			"*** End Patch",
		nil,
		"Invalid patch hunk on line 4: Unexpected line found in update hunk: 'bad'. "+
			"Every line should start with ' ' (context line), '+' (added line), or "+
			"'-' (removed line)")

	// The patch wrapped in a bash heredoc should parse leniently.
	testParsePatch(t,
		"<<EOF\n"+
			"*** Begin Patch\n"+
			"*** Update File: file2.py\n"+
			" import foo\n"+
			"+bar\n"+
			"*** End Patch\n"+
			"EOF",
		[]patchHunk{
			{
				op:   updateFileOp,
				path: "file2.py",
				chunks: []updateChunk{
					{
						oldLines: []string{"import foo"},
						newLines: []string{"import foo", "bar"},
					},
				},
			},
		}, "")
}

func testApplyPatch(t *testing.T, patch, output, errMsg string) {
	t.Helper()

	ret, err := runApplyPatch(t, patch)
	if errMsg != "" {
		if err == nil {
			t.Errorf("applyPatch(%q) did not fail; expected %q", patch, errMsg)
		} else if err.Error() != errMsg {
			t.Errorf("applyPatch(%q) failed with %q; expected %q", patch, err, errMsg)
		}
	} else if err != nil {
		t.Errorf("applyPatch(%q) failed with %s", patch, err)
	} else if ret != output {
		t.Errorf("applyPatch(%q) got %q; expected %q", patch, ret, output)
	}
}

func testFileContents(t *testing.T, path, contents string) {
	t.Helper()

	buf, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("ReadFile(%q) failed with %s", path, err)
	} else if string(buf) != contents {
		t.Errorf("ReadFile(%q) got %q; expected %q", path, string(buf), contents)
	}
}

func TestApplyPatchAddFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "add.txt")
	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Add File: %s\n+ab\n+cd", path)),
		fmt.Sprintf("Success. Updated the following files:\nA %s\n", path), "")
	testFileContents(t, path, "ab\ncd\n")
}

func TestApplyPatchAddFileCreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "add.txt")
	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Add File: %s\n+content", path)),
		fmt.Sprintf("Success. Updated the following files:\nA %s\n", path), "")
	testFileContents(t, path, "content\n")
}

func TestApplyPatchDeleteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "del.txt")
	err := os.WriteFile(path, []byte("x"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Delete File: %s", path)),
		fmt.Sprintf("Success. Updated the following files:\nD %s\n", path), "")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s was not deleted", path)
	}
}

func TestApplyPatchUpdateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.txt")
	err := os.WriteFile(path, []byte("foo\nbar\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n@@\n foo\n-bar\n+baz", path)),
		fmt.Sprintf("Success. Updated the following files:\nM %s\n", path), "")
	testFileContents(t, path, "foo\nbaz\n")
}

func TestApplyPatchUpdateFileMove(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dest := filepath.Join(dir, "dst.txt")
	err := os.WriteFile(src, []byte("line\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n*** Move to: %s\n@@\n-line\n+line2",
			src, dest)),
		fmt.Sprintf("Success. Updated the following files:\nM %s\n", dest), "")
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("%s was not removed", src)
	}
	testFileContents(t, dest, "line2\n")
}

func TestApplyPatchMultipleUpdateChunks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.txt")
	err := os.WriteFile(path, []byte("foo\nbar\nbaz\nqux\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf(
			"*** Update File: %s\n@@\n foo\n-bar\n+BAR\n@@\n baz\n-qux\n+QUX", path)),
		fmt.Sprintf("Success. Updated the following files:\nM %s\n", path), "")
	testFileContents(t, path, "foo\nBAR\nbaz\nQUX\n")
}

func TestApplyPatchInterleavedChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interleaved.txt")
	err := os.WriteFile(path, []byte("a\nb\nc\nd\ne\nf\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n"+
			"@@\n a\n-b\n+B\n"+
			"@@\n c\n d\n-e\n+E\n"+
			"@@\n f\n+g\n*** End of File", path)),
		fmt.Sprintf("Success. Updated the following files:\nM %s\n", path), "")
	testFileContents(t, path, "a\nB\nc\nd\nE\nf\ng\n")
}

func TestApplyPatchPureAdditionThenRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panic.txt")
	err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n"+
			"@@\n+after-context\n+second-line\n"+
			"@@\n line1\n-line2\n-line3\n+line2-replacement", path)),
		fmt.Sprintf("Success. Updated the following files:\nM %s\n", path), "")
	testFileContents(t, path, "line1\nline2-replacement\nafter-context\nsecond-line\n")
}

func TestApplyPatchUnicodeDash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unicode.py")

	// The original line contains EN DASH (U+2013) and NON-BREAKING HYPHEN (U+2011); the
	// patch uses plain ASCII dash / hyphen and should still match.
	err := os.WriteFile(path,
		[]byte("import asyncio  # local import – avoids top‑level dep\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n@@\n"+
			"-import asyncio  # local import - avoids top-level dep\n"+
			"+import asyncio  # HELLO", path)),
		fmt.Sprintf("Success. Updated the following files:\nM %s\n", path), "")
	testFileContents(t, path, "import asyncio  # HELLO\n")
}

func TestApplyPatchChangeContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "context.py")
	err := os.WriteFile(path,
		[]byte("class A:\n    def f():\n        pass\nclass B:\n    def f():\n        pass\n"),
		0o644)
	if err != nil {
		t.Fatal(err)
	}

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n@@ class B:\n     def f():\n"+
			"-        pass\n+        return 123", path)),
		fmt.Sprintf("Success. Updated the following files:\nM %s\n", path), "")
	testFileContents(t, path,
		"class A:\n    def f():\n        pass\nclass B:\n    def f():\n        return 123\n")
}

func TestApplyPatchErrors(t *testing.T) {
	dir := t.TempDir()

	testApplyPatch(t, "*** Begin Patch\n*** End Patch", "", "No files were modified.")

	path := filepath.Join(dir, "missing-context.txt")
	err := os.WriteFile(path, []byte("foo\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n@@\n-bar\n+baz", path)), "",
		fmt.Sprintf("Failed to find expected lines in %s:\nbar", path))

	testApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n@@ def missing():\n-foo\n+baz", path)),
		"", fmt.Sprintf("Failed to find context 'def missing():' in %s", path))

	missing := filepath.Join(dir, "missing.txt")
	ret, err := runApplyPatch(t,
		wrapPatch(fmt.Sprintf("*** Update File: %s\n@@\n-foo\n+bar", missing)))
	if err == nil {
		t.Errorf("applyPatch did not fail; got %q", ret)
	} else if !strings.HasPrefix(err.Error(),
		fmt.Sprintf("Failed to read file to update %s", missing)) {

		t.Errorf("applyPatch failed with %q", err)
	}

	ret, err = runApplyPatch(t, wrapPatch(fmt.Sprintf("*** Delete File: %s", missing)))
	if err == nil {
		t.Errorf("applyPatch did not fail; got %q", ret)
	} else if !strings.HasPrefix(err.Error(),
		fmt.Sprintf("Failed to delete file %s", missing)) {

		t.Errorf("applyPatch failed with %q", err)
	}
}
