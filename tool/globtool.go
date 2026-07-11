package tool

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type globArgs struct {
	Pattern string `json:"pattern" gait:"glob pattern for matching files"`
	Path    string `json:"path,omitempty" gait:"directory to search; defaults to current directory"`
}

// doubleStarMatch matches a slash-separated glob pattern against a
// slash-separated path, where "**" matches any number of path segments
// (including zero). Non-"**" segments are matched with filepath.Match.
func doubleStarMatch(patternParts, nameParts []string) bool {
	if len(patternParts) == 0 {
		return len(nameParts) == 0
	}

	if patternParts[0] == "**" {
		for i := 0; i <= len(nameParts); i += 1 {
			if doubleStarMatch(patternParts[1:], nameParts[i:]) {
				return true
			}
		}
		return false
	}

	if len(nameParts) == 0 {
		return false
	}

	matched, err := filepath.Match(patternParts[0], nameParts[0])
	if err != nil || !matched {
		return false
	}
	return doubleStarMatch(patternParts[1:], nameParts[1:])
}

func globMatch(pattern, name string) bool {
	return doubleStarMatch(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

type globMatchInfo struct {
	path    string
	modTime int64
}

func glob(ctx context.Context, buf []byte) (string, error) {
	var args globArgs
	if err := json.Unmarshal(buf, &args); err != nil {
		return "", err
	}

	root := args.Path
	if root == "" {
		root = "."
	}

	pattern := filepath.ToSlash(args.Pattern)

	var matches []globMatchInfo
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if !globMatch(pattern, rel) {
			return nil
		}

		var modTime int64
		if fi, err := d.Info(); err == nil {
			modTime = fi.ModTime().UnixNano()
		}
		matches = append(matches, globMatchInfo{path: path, modTime: modTime})
		return nil
	})
	if err != nil {
		return "", err
	}

	// Sort by modification time, most recently modified first.
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	if len(matches) == 0 {
		return "No files found.", nil
	}

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m.path)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func Glob() Tool {
	return Tool{
		Name: "glob",
		Description: "Fast file pattern matching tool that works with any codebase size. " +
			"Supports glob patterns like \"**/*.js\" or \"src/**/*.ts\". " +
			"Returns matching file paths sorted by modification time.",
		Func:   glob,
		Schema: MustToolSchema[globArgs](),
	}
}
