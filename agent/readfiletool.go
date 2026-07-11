package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/leftmike/gait/model"
)

const (
	// readFileDefaultLimit is the maximum number of lines read_file returns
	// when no limit is given, matching Claude Code's read_file tool.
	readFileDefaultLimit = 2000

	// readFileMaxLineLen is the maximum number of characters emitted for a
	// single line; longer lines are truncated.
	readFileMaxLineLen = 2000
)

type readFileArgs struct {
	Path   string `json:"path" gait:"the path of the file to read"`
	Offset int    `json:"offset,omitempty" gait:"the line number to start reading from (1-based); defaults to the start of the file"`
	Limit  int    `json:"limit,omitempty" gait:"the maximum number of lines to read; defaults to 2000"`
}

func readFile(ctx context.Context, buf []byte) (string, error) {
	var args readFileArgs
	if err := json.Unmarshal(buf, &args); err != nil {
		return "", err
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", err
	}

	return formatReadFile(string(data), args.Offset, args.Limit), nil
}

// formatReadFile renders file content the way Claude Code's read_file tool
// does for text files: a `cat -n` style listing where each line is prefixed
// with its 1-based line number, right-justified in a six-character field, and
// a tab. Reading starts at offset (1-based, defaults to the first line) and
// returns at most limit lines (defaults to readFileDefaultLimit). Lines longer
// than readFileMaxLineLen characters are truncated.
func formatReadFile(content string, offset, limit int) string {
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 {
		limit = readFileDefaultLimit
	}

	// An empty file has no lines to number.
	if content == "" {
		return ""
	}

	// A single trailing newline terminates the last line rather than
	// introducing an empty one, matching cat -n.
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")

	var sb strings.Builder
	start := offset - 1
	for i := start; i < len(lines) && i < start+limit; i += 1 {
		line := lines[i]
		if len([]rune(line)) > readFileMaxLineLen {
			line = string([]rune(line)[:readFileMaxLineLen])
		}
		fmt.Fprintf(&sb, "%6d\t%s\n", i+1, line)
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

func (ag *Agent) AddReadFileTool() {
	if _, ok := ag.Tools["read_file"]; !ok {
		ag.AddTool("read_file",
			"Reads a text file from the local filesystem and returns its contents in "+
				"`cat -n` format, with each line prefixed by its 1-based line number. "+
				"Reads up to 2000 lines by default; use offset and limit to read a "+
				"specific range of a large file. Lines longer than 2000 characters are "+
				"truncated.",
			readFile, model.MustToolSchema[readFileArgs]())
	}
}
