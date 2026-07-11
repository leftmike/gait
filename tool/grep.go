package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const maxGrepFileSize = 1024 * 1024 * 8

type grepArgs struct {
	Pattern     string `json:"pattern" gait:"the regular expression pattern to search for"`
	Path        string `json:"path,omitempty" gait:"file or directory to search in; defaults to the current working directory"`
	Glob        string `json:"glob,omitempty" gait:"glob pattern to filter which files are searched (e.g. *.go)"`
	OutputMode  string `json:"output_mode,omitempty" gait:"one of: content / files_with_matches (default) / count"`
	IgnoreCase  bool   `json:"-i,omitempty" gait:"case insensitive search"`
	LineNumbers bool   `json:"-n,omitempty" gait:"show line numbers (content mode only)"`
	After       int    `json:"-A,omitempty" gait:"lines of context to show after each match (content mode only)"`
	Before      int    `json:"-B,omitempty" gait:"lines of context to show before each match (content mode only)"`
	Context     int    `json:"-C,omitempty" gait:"lines of context to show before and after each match (content mode only)"`
	Multiline   bool   `json:"multiline,omitempty" gait:"allow the pattern to span multiple lines"`
	HeadLimit   int    `json:"head_limit,omitempty" gait:"limit output to the first N entries; 0 means unlimited"`
}

func matchGlobFilter(pattern, rel string) bool {
	if pattern == "" {
		return true
	}
	if strings.Contains(pattern, "/") {
		return globMatch(pattern, rel)
	}
	return globMatch(pattern, filepath.Base(rel))
}

// grepFile returns the 1-based line numbers in content that match re. When
// multiline is set, matches may span lines and every line a match touches is
// reported.
func grepFileLines(re *regexp.Regexp, content string, multiline bool) []int {
	lines := strings.Split(content, "\n")

	if !multiline {
		var out []int
		for i, ln := range lines {
			if re.MatchString(ln) {
				out = append(out, i+1)
			}
		}
		return out
	}

	// Map byte offsets to line numbers, then collect every line each match
	// overlaps.
	lineStart := make([]int, len(lines))
	off := 0
	for i, ln := range lines {
		lineStart[i] = off
		off += len(ln) + 1 // account for the split '\n'
	}
	lineOf := func(pos int) int {
		i := sort.Search(len(lineStart), func(k int) bool { return lineStart[k] > pos }) - 1
		if i < 0 {
			i = 0
		}
		return i
	}

	seen := map[int]bool{}
	for _, m := range re.FindAllStringIndex(content, -1) {
		end := m[1]
		if end > m[0] {
			end-- // don't spill onto the line after a trailing match
		}
		for l := lineOf(m[0]); l <= lineOf(end); l += 1 {
			seen[l+1] = true
		}
	}

	out := make([]int, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Ints(out)
	return out
}

func grep(ctx context.Context, buf []byte) (string, error) {
	var args grepArgs
	if err := json.Unmarshal(buf, &args); err != nil {
		return "", err
	}

	expr := args.Pattern
	var flags string
	if args.IgnoreCase {
		flags += "i"
	}
	if args.Multiline {
		flags += "s" // let '.' match newlines so patterns can span lines
	}
	if flags != "" {
		expr = "(?" + flags + ")" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}

	root := args.Path
	if root == "" {
		root = "."
	}

	// Collect the files to search.
	var files []string
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				rel = path
			}
			if matchGlobFilter(args.Glob, filepath.ToSlash(rel)) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		sort.Strings(files)
	} else {
		files = []string{root}
	}

	mode := args.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}

	before, after := args.Before, args.After
	if args.Context > 0 {
		before, after = args.Context, args.Context
	}

	var lines []string // output lines, before applying head_limit

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil || len(data) > maxGrepFileSize || isBinary(data) {
			continue
		}
		content := string(data)
		matched := grepFileLines(re, content, args.Multiline)
		if len(matched) == 0 {
			continue
		}

		switch mode {
		case "files_with_matches":
			lines = append(lines, path)
		case "count":
			lines = append(lines, fmt.Sprintf("%s:%d", path, len(matched)))
		case "content":
			lines = append(lines, grepContentLines(path, content, matched, before, after,
				args.LineNumbers)...)
		default:
			return "", fmt.Errorf("invalid output_mode: %s", mode)
		}
	}

	if len(lines) == 0 {
		return "No matches found.", nil
	}
	if args.HeadLimit > 0 && len(lines) > args.HeadLimit {
		lines = lines[:args.HeadLimit]
	}
	return strings.Join(lines, "\n"), nil
}

// grepContentLines renders the matching lines of a single file (with optional
// context) in a ripgrep-like "path:line:text" / "path-line-text" form.
func grepContentLines(path, content string, matched []int, before, after int,
	lineNumbers bool) []string {

	fileLines := strings.Split(content, "\n")
	isMatch := make(map[int]bool, len(matched))
	for _, l := range matched {
		isMatch[l] = true
	}

	// Build the ordered set of line numbers to emit, merging context windows.
	emit := map[int]bool{}
	for _, l := range matched {
		for n := l - before; n <= l+after; n += 1 {
			if n >= 1 && n <= len(fileLines) {
				emit[n] = true
			}
		}
	}
	ordered := make([]int, 0, len(emit))
	for n := range emit {
		ordered = append(ordered, n)
	}
	sort.Ints(ordered)

	var out []string
	prev := 0
	for _, n := range ordered {
		if prev != 0 && n != prev+1 {
			out = append(out, "--")
		}
		prev = n

		sep := "-"
		if isMatch[n] {
			sep = ":"
		}
		if lineNumbers {
			out = append(out, fmt.Sprintf("%s%s%d%s%s", path, sep, n, sep, fileLines[n-1]))
		} else {
			out = append(out, fmt.Sprintf("%s%s%s", path, sep, fileLines[n-1]))
		}
	}
	return out
}

var Grep = Tool{
	Name: "grep",
	Description: "Fast content search over text files using regular expressions. Supports " +
		"filtering files by glob, three output modes (content, " +
		"files_with_matches, count), case-insensitive and multiline matching, " +
		"and context lines. Binary files are skipped.",
	Func:   grep,
	Schema: MustToolSchema[grepArgs](),
}
