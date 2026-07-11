// The apply_patch tool, identical to the apply_patch tool in OpenAI Codex.
// Ported from https://github.com/openai/codex codex-rs/apply-patch (and the
// tool description from codex-rs/core/src/tool_apply_patch.rs).
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"
)

const applyPatchDescription = "Use the `apply_patch` tool to edit files.\n" +
	"Your patch language is a stripped‑down, file‑oriented diff format designed to be easy to parse and safe to apply. You can think of it as a high‑level envelope:\n" +
	"\n" +
	"*** Begin Patch\n" +
	"[ one or more file sections ]\n" +
	"*** End Patch\n" +
	"\n" +
	"Within that envelope, you get a sequence of file operations.\n" +
	"You MUST include a header to specify the action you are taking.\n" +
	"Each operation starts with one of three headers:\n" +
	"\n" +
	"*** Add File: <path> - create a new file. Every following line is a + line (the initial contents).\n" +
	"*** Delete File: <path> - remove an existing file. Nothing follows.\n" +
	"*** Update File: <path> - patch an existing file in place (optionally with a rename).\n" +
	"\n" +
	"May be immediately followed by *** Move to: <new path> if you want to rename the file.\n" +
	"Then one or more “hunks”, each introduced by @@ (optionally followed by a hunk header).\n" +
	"Within a hunk each line starts with:\n" +
	"\n" +
	"For instructions on [context_before] and [context_after]:\n" +
	"- By default, show 3 lines of code immediately above and 3 lines immediately below each change. If a change is within 3 lines of a previous change, do NOT duplicate the first change’s [context_after] lines in the second change’s [context_before] lines.\n" +
	"- If 3 lines of context is insufficient to uniquely identify the snippet of code within the file, use the @@ operator to indicate the class or function to which the snippet belongs. For instance, we might have:\n" +
	"@@ class BaseClass\n" +
	"[3 lines of pre-context]\n" +
	"- [old_code]\n" +
	"+ [new_code]\n" +
	"[3 lines of post-context]\n" +
	"\n" +
	"- If a code block is repeated so many times in a class or function such that even a single `@@` statement and 3 lines of context cannot uniquely identify the snippet of code, you can use multiple `@@` statements to jump to the right context. For instance:\n" +
	"\n" +
	"@@ class BaseClass\n" +
	"@@ \t def method():\n" +
	"[3 lines of pre-context]\n" +
	"- [old_code]\n" +
	"+ [new_code]\n" +
	"[3 lines of post-context]\n" +
	"\n" +
	"The full grammar definition is below:\n" +
	"Patch := Begin { FileOp } End\n" +
	"Begin := \"*** Begin Patch\" NEWLINE\n" +
	"End := \"*** End Patch\" NEWLINE\n" +
	"FileOp := AddFile | DeleteFile | UpdateFile\n" +
	"AddFile := \"*** Add File: \" path NEWLINE { \"+\" line NEWLINE }\n" +
	"DeleteFile := \"*** Delete File: \" path NEWLINE\n" +
	"UpdateFile := \"*** Update File: \" path NEWLINE [ MoveTo ] { Hunk }\n" +
	"MoveTo := \"*** Move to: \" newPath NEWLINE\n" +
	"Hunk := \"@@\" [ header ] NEWLINE { HunkLine } [ \"*** End of File\" NEWLINE ]\n" +
	"HunkLine := (\" \" | \"-\" | \"+\") text NEWLINE\n" +
	"\n" +
	"A full patch can combine several operations:\n" +
	"\n" +
	"*** Begin Patch\n" +
	"*** Add File: hello.txt\n" +
	"+Hello world\n" +
	"*** Update File: src/app.py\n" +
	"*** Move to: src/main.py\n" +
	"@@ def greet():\n" +
	"-print(\"Hi\")\n" +
	"+print(\"Hello, world!\")\n" +
	"*** Delete File: obsolete.txt\n" +
	"*** End Patch\n" +
	"\n" +
	"It is important to remember:\n" +
	"\n" +
	"- You must include a header with your intended action (Add/Delete/Update)\n" +
	"- You must prefix new lines with `+` even when creating a new file\n" +
	"- File references can only be relative, NEVER ABSOLUTE.\n"

type applyPatchArgs struct {
	Input string `json:"input" gait:"The entire contents of the apply_patch command"`
}

func applyPatch(ctx context.Context, buf []byte) (string, error) {
	var args applyPatchArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	hunks, err := parsePatch(args.Input)
	if err != nil {
		return "", err
	}

	affected, err := applyHunks(hunks)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("Success. Updated the following files:\n")
	for _, path := range affected.added {
		fmt.Fprintf(&sb, "A %s\n", path)
	}
	for _, path := range affected.modified {
		fmt.Fprintf(&sb, "M %s\n", path)
	}
	for _, path := range affected.deleted {
		fmt.Fprintf(&sb, "D %s\n", path)
	}
	return sb.String(), nil
}

var ApplyPatch = Tool{
	Name:        "apply_patch",
	Description: applyPatchDescription,
	Func:        applyPatch,
	Schema:      MustToolSchema[applyPatchArgs](),
}

// The patch format is:
//
//	start: begin_patch hunk+ end_patch
//	begin_patch: "*** Begin Patch" LF
//	end_patch: "*** End Patch" LF?
//
//	hunk: add_hunk | delete_hunk | update_hunk
//	add_hunk: "*** Add File: " filename LF add_line+
//	delete_hunk: "*** Delete File: " filename LF
//	update_hunk: "*** Update File: " filename LF change_move? change?
//	filename: /(.+)/
//	add_line: "+" /(.+)/ LF -> line
//
//	change_move: "*** Move to: " filename LF
//	change: (change_context | change_line)+ eof_line?
//	change_context: ("@@" | "@@ " /(.+)/) LF
//	change_line: ("+" | "-" | " ") /(.+)/ LF
//	eof_line: "*** End of File" LF
//
// The parser below is a little more lenient than the explicit spec and allows for
// leading/trailing whitespace around patch markers.
const (
	beginPatchMarker         = "*** Begin Patch"
	endPatchMarker           = "*** End Patch"
	addFileMarker            = "*** Add File: "
	deleteFileMarker         = "*** Delete File: "
	updateFileMarker         = "*** Update File: "
	moveToMarker             = "*** Move to: "
	patchEOFMarker           = "*** End of File"
	changeContextMarker      = "@@ "
	emptyChangeContextMarker = "@@"
)

type hunkOp int

const (
	addFileOp hunkOp = iota
	deleteFileOp
	updateFileOp
)

type patchHunk struct {
	op       hunkOp
	path     string
	contents string        // addFileOp: the initial contents of the file
	movePath string        // updateFileOp: optional path to rename the file to
	chunks   []updateChunk // updateFileOp
}

type updateChunk struct {
	// A single line of context used to narrow down the position of the chunk
	// (this is usually a class, method, or function definition.)
	changeContext    string
	hasChangeContext bool

	// A contiguous block of lines that should be replaced with newLines.
	// oldLines must occur strictly after changeContext.
	oldLines []string
	newLines []string

	// If true, oldLines must occur at the end of the source file.
	isEndOfFile bool
}

func invalidPatchError(msg string) error {
	return fmt.Errorf("Invalid patch: %s", msg)
}

func invalidHunkError(msg string, lineNumber int) error {
	return fmt.Errorf("Invalid patch hunk on line %d: %s", lineNumber, msg)
}

func parsePatch(patch string) ([]patchHunk, error) {
	var lines []string
	if trimmed := strings.TrimSpace(patch); trimmed != "" {
		lines = strings.Split(trimmed, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimSuffix(line, "\r")
		}
	}

	err := checkPatchBoundariesStrict(lines)
	if err != nil {
		// Some models wrap the patch in a bash heredoc (apply_patch <<'EOF' ... EOF);
		// leniently strip the heredoc markers and try again.
		lines, err = checkPatchBoundariesLenient(lines, err)
		if err != nil {
			return nil, err
		}
	}

	var hunks []patchHunk
	remainingLines := lines[1 : len(lines)-1]
	lineNumber := 2
	for len(remainingLines) > 0 {
		hunk, hunkLines, err := parseOneHunk(remainingLines, lineNumber)
		if err != nil {
			return nil, err
		}
		hunks = append(hunks, hunk)
		lineNumber += hunkLines
		remainingLines = remainingLines[hunkLines:]
	}
	return hunks, nil
}

func checkPatchBoundariesStrict(lines []string) error {
	if len(lines) > 0 && lines[0] != beginPatchMarker {
		return invalidPatchError("The first line of the patch must be '*** Begin Patch'")
	} else if len(lines) < 2 || lines[len(lines)-1] != endPatchMarker {
		return invalidPatchError("The last line of the patch must be '*** End Patch'")
	}
	return nil
}

func checkPatchBoundariesLenient(lines []string, origErr error) ([]string, error) {
	if len(lines) >= 4 {
		first, last := lines[0], lines[len(lines)-1]
		if (first == "<<EOF" || first == "<<'EOF'" || first == `<<"EOF"`) &&
			strings.HasSuffix(last, "EOF") {

			innerLines := lines[1 : len(lines)-1]
			err := checkPatchBoundariesStrict(innerLines)
			if err != nil {
				return nil, err
			}
			return innerLines, nil
		}
	}
	return nil, origErr
}

// Attempts to parse a single hunk from the start of lines. Returns the parsed hunk and
// the number of lines parsed.
func parseOneHunk(lines []string, lineNumber int) (patchHunk, int, error) {
	// Be tolerant of extra padding around marker strings.
	firstLine := strings.TrimSpace(lines[0])
	if path, ok := strings.CutPrefix(firstLine, addFileMarker); ok {
		// Add File
		var contents strings.Builder
		parsedLines := 1
		for _, addLine := range lines[1:] {
			lineToAdd, ok := strings.CutPrefix(addLine, "+")
			if !ok {
				break
			}
			contents.WriteString(lineToAdd)
			contents.WriteByte('\n')
			parsedLines += 1
		}
		return patchHunk{op: addFileOp, path: path, contents: contents.String()},
			parsedLines, nil
	} else if path, ok := strings.CutPrefix(firstLine, deleteFileMarker); ok {
		// Delete File
		return patchHunk{op: deleteFileOp, path: path}, 1, nil
	} else if path, ok := strings.CutPrefix(firstLine, updateFileMarker); ok {
		// Update File
		remainingLines := lines[1:]
		parsedLines := 1

		// Optional: move file line
		var movePath string
		if len(remainingLines) > 0 {
			if mp, ok := strings.CutPrefix(remainingLines[0], moveToMarker); ok {
				movePath = mp
				remainingLines = remainingLines[1:]
				parsedLines += 1
			}
		}

		var chunks []updateChunk
		for len(remainingLines) > 0 {
			// Skip over any completely blank lines that may separate chunks.
			if strings.TrimSpace(remainingLines[0]) == "" {
				parsedLines += 1
				remainingLines = remainingLines[1:]
				continue
			}

			// Stop once we reach the next special marker header.
			if strings.HasPrefix(remainingLines[0], "***") {
				break
			}

			chunk, chunkLines, err := parseUpdateChunk(remainingLines,
				lineNumber+parsedLines, len(chunks) == 0)
			if err != nil {
				return patchHunk{}, 0, err
			}
			chunks = append(chunks, chunk)
			parsedLines += chunkLines
			remainingLines = remainingLines[chunkLines:]
		}

		if len(chunks) == 0 {
			return patchHunk{}, 0, invalidHunkError(
				fmt.Sprintf("Update file hunk for path '%s' is empty", path), lineNumber)
		}

		return patchHunk{op: updateFileOp, path: path, movePath: movePath, chunks: chunks},
			parsedLines, nil
	}

	return patchHunk{}, 0, invalidHunkError(
		fmt.Sprintf("'%s' is not a valid hunk header. Valid hunk headers: "+
			"'*** Add File: {path}', '*** Delete File: {path}', '*** Update File: {path}'",
			firstLine), lineNumber)
}

func parseUpdateChunk(lines []string, lineNumber int, allowMissingContext bool) (updateChunk,
	int, error) {

	if len(lines) == 0 {
		return updateChunk{}, 0,
			invalidHunkError("Update hunk does not contain any lines", lineNumber)
	}

	// If we see an explicit context marker @@ or @@ <context>, consume it; otherwise,
	// optionally allow treating the chunk as starting directly with diff lines.
	var chunk updateChunk
	startIndex := 0
	if lines[0] == emptyChangeContextMarker {
		startIndex = 1
	} else if context, ok := strings.CutPrefix(lines[0], changeContextMarker); ok {
		chunk.changeContext = context
		chunk.hasChangeContext = true
		startIndex = 1
	} else if !allowMissingContext {
		return updateChunk{}, 0, invalidHunkError(
			fmt.Sprintf("Expected update hunk to start with a @@ context marker, got: '%s'",
				lines[0]), lineNumber)
	}
	if startIndex >= len(lines) {
		return updateChunk{}, 0,
			invalidHunkError("Update hunk does not contain any lines", lineNumber+1)
	}

	parsedLines := 0
loop:
	for _, line := range lines[startIndex:] {
		if line == patchEOFMarker {
			if parsedLines == 0 {
				return updateChunk{}, 0,
					invalidHunkError("Update hunk does not contain any lines", lineNumber+1)
			}
			chunk.isEndOfFile = true
			parsedLines += 1
			break
		}

		if line == "" {
			// Interpret this as an empty context line.
			chunk.oldLines = append(chunk.oldLines, "")
			chunk.newLines = append(chunk.newLines, "")
		} else {
			switch line[0] {
			case ' ':
				chunk.oldLines = append(chunk.oldLines, line[1:])
				chunk.newLines = append(chunk.newLines, line[1:])
			case '+':
				chunk.newLines = append(chunk.newLines, line[1:])
			case '-':
				chunk.oldLines = append(chunk.oldLines, line[1:])
			default:
				if parsedLines == 0 {
					return updateChunk{}, 0, invalidHunkError(
						fmt.Sprintf("Unexpected line found in update hunk: '%s'. "+
							"Every line should start with ' ' (context line), "+
							"'+' (added line), or '-' (removed line)", line), lineNumber+1)
				}
				// Assume this is the start of the next hunk.
				break loop
			}
		}
		parsedLines += 1
	}

	return chunk, parsedLines + startIndex, nil
}

type affectedPaths struct {
	added    []string
	modified []string
	deleted  []string
}

// Apply the hunks to the filesystem, returning which files were added, modified, or
// deleted.
func applyHunks(hunks []patchHunk) (affectedPaths, error) {
	if len(hunks) == 0 {
		return affectedPaths{}, fmt.Errorf("No files were modified.")
	}

	var affected affectedPaths
	for _, hunk := range hunks {
		switch hunk.op {
		case addFileOp:
			err := makeParentDirs(hunk.path)
			if err != nil {
				return affectedPaths{}, err
			}
			err = os.WriteFile(hunk.path, []byte(hunk.contents), 0o644)
			if err != nil {
				return affectedPaths{},
					fmt.Errorf("Failed to write file %s: %s", hunk.path, err)
			}
			affected.added = append(affected.added, hunk.path)

		case deleteFileOp:
			err := os.Remove(hunk.path)
			if err != nil {
				return affectedPaths{},
					fmt.Errorf("Failed to delete file %s: %s", hunk.path, err)
			}
			affected.deleted = append(affected.deleted, hunk.path)

		case updateFileOp:
			newContents, err := deriveNewContentsFromChunks(hunk.path, hunk.chunks)
			if err != nil {
				return affectedPaths{}, err
			}
			if hunk.movePath != "" {
				err = makeParentDirs(hunk.movePath)
				if err != nil {
					return affectedPaths{}, err
				}
				err = os.WriteFile(hunk.movePath, []byte(newContents), 0o644)
				if err != nil {
					return affectedPaths{},
						fmt.Errorf("Failed to write file %s: %s", hunk.movePath, err)
				}
				err = os.Remove(hunk.path)
				if err != nil {
					return affectedPaths{},
						fmt.Errorf("Failed to remove original %s: %s", hunk.path, err)
				}
				affected.modified = append(affected.modified, hunk.movePath)
			} else {
				err = os.WriteFile(hunk.path, []byte(newContents), 0o644)
				if err != nil {
					return affectedPaths{},
						fmt.Errorf("Failed to write file %s: %s", hunk.path, err)
				}
				affected.modified = append(affected.modified, hunk.path)
			}
		}
	}
	return affected, nil
}

func makeParentDirs(path string) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			return fmt.Errorf("Failed to create parent directories for %s: %s", path, err)
		}
	}
	return nil
}

// Return the new contents of the file at path after applying the chunks to it.
func deriveNewContentsFromChunks(path string, chunks []updateChunk) (string, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("Failed to read file to update %s: %s", path, err)
	}

	originalLines := strings.Split(string(buf), "\n")

	// Drop the trailing empty element that results from the final newline so that line
	// counts match the behaviour of standard diff.
	if len(originalLines) > 0 && originalLines[len(originalLines)-1] == "" {
		originalLines = originalLines[:len(originalLines)-1]
	}

	replacements, err := computeReplacements(originalLines, path, chunks)
	if err != nil {
		return "", err
	}

	newLines := applyReplacements(originalLines, replacements)
	if len(newLines) == 0 || newLines[len(newLines)-1] != "" {
		newLines = append(newLines, "")
	}
	return strings.Join(newLines, "\n"), nil
}

// Each replacement replaces the oldLen lines starting at startIndex with newLines.
type replacement struct {
	startIndex int
	oldLen     int
	newLines   []string
}

// Compute a list of replacements needed to transform originalLines into the new lines,
// given the patch chunks.
func computeReplacements(originalLines []string, path string,
	chunks []updateChunk) ([]replacement, error) {

	var replacements []replacement
	lineIndex := 0

	for _, chunk := range chunks {
		// If a chunk has a change context, find it and continue from there.
		if chunk.hasChangeContext {
			idx := seekSequence(originalLines, []string{chunk.changeContext}, lineIndex,
				false)
			if idx < 0 {
				return nil, fmt.Errorf("Failed to find context '%s' in %s",
					chunk.changeContext, path)
			}
			lineIndex = idx + 1
		}

		if len(chunk.oldLines) == 0 {
			// Pure addition (no old lines). Add them at the end or just before the
			// final empty line if one exists.
			insertionIndex := len(originalLines)
			if insertionIndex > 0 && originalLines[insertionIndex-1] == "" {
				insertionIndex -= 1
			}
			replacements = append(replacements,
				replacement{startIndex: insertionIndex, newLines: chunk.newLines})
			continue
		}

		// Otherwise, try to match the existing lines in the file with the old lines from
		// the chunk. In many real-world diffs the last element of oldLines is an empty
		// string representing the terminating newline of the region being replaced. This
		// sentinel is not present in originalLines because the trailing empty line was
		// stripped above. If a direct search fails and the pattern ends with an empty
		// string, retry without that final element so that modifications touching the
		// end-of-file can be located reliably.
		pattern := chunk.oldLines
		newLines := chunk.newLines
		found := seekSequence(originalLines, pattern, lineIndex, chunk.isEndOfFile)
		if found < 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			found = seekSequence(originalLines, pattern, lineIndex, chunk.isEndOfFile)
		}

		if found < 0 {
			return nil, fmt.Errorf("Failed to find expected lines in %s:\n%s", path,
				strings.Join(chunk.oldLines, "\n"))
		}
		replacements = append(replacements,
			replacement{startIndex: found, oldLen: len(pattern), newLines: newLines})
		lineIndex = found + len(pattern)
	}

	sort.SliceStable(replacements,
		func(i, j int) bool {
			return replacements[i].startIndex < replacements[j].startIndex
		})

	return replacements, nil
}

// Apply the replacements to lines, returning the modified file contents as a slice of
// lines.
func applyReplacements(lines []string, replacements []replacement) []string {
	// Apply replacements in descending order so that earlier replacements don't shift
	// the positions of later ones.
	for i := len(replacements) - 1; i >= 0; i -= 1 {
		rep := replacements[i]
		end := min(rep.startIndex+rep.oldLen, len(lines))
		start := min(rep.startIndex, end)
		lines = slices.Insert(slices.Delete(lines, start, end), start, rep.newLines...)
	}
	return lines
}

// Attempt to find the sequence of pattern lines within lines beginning at or after start.
// Returns the starting index of the match or -1 if not found. Matches are attempted with
// decreasing strictness: exact match, then ignoring trailing whitespace, then ignoring
// leading and trailing whitespace, then after normalizing common Unicode punctuation to
// ASCII equivalents. When eof is true, first try matching at the end-of-file (so that
// patterns intended to match file endings are applied at the end), falling back to
// searching from start if needed.
func seekSequence(lines []string, pattern []string, start int, eof bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}

	searchStart := start
	if eof {
		searchStart = len(lines) - len(pattern)
	}

	seek := func(match func(line, pat string) bool) int {
		for i := searchStart; i <= len(lines)-len(pattern); i += 1 {
			ok := true
			for j, pat := range pattern {
				if !match(lines[i+j], pat) {
					ok = false
					break
				}
			}
			if ok {
				return i
			}
		}
		return -1
	}

	// Exact match first.
	if idx := seek(func(line, pat string) bool { return line == pat }); idx >= 0 {
		return idx
	}
	// Then ignore trailing whitespace.
	if idx := seek(
		func(line, pat string) bool {
			return strings.TrimRightFunc(line, unicode.IsSpace) ==
				strings.TrimRightFunc(pat, unicode.IsSpace)
		}); idx >= 0 {

		return idx
	}
	// Then trim both sides to allow more lenience.
	if idx := seek(
		func(line, pat string) bool {
			return strings.TrimSpace(line) == strings.TrimSpace(pat)
		}); idx >= 0 {

		return idx
	}
	// Finally, the most permissive pass: attempt to match after normalizing common
	// Unicode punctuation to ASCII equivalents so that diffs authored with plain ASCII
	// can still be applied to source files that contain typographic dashes, quotes, etc.
	return seek(
		func(line, pat string) bool {
			return normalizeSeekLine(line) == normalizeSeekLine(pat)
		})
}

func normalizeSeekLine(s string) string {
	return strings.Map(
		func(r rune) rune {
			switch r {
			// Various dash / hyphen code-points.
			case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
				return '-'
			// Fancy single quotes.
			case '\u2018', '\u2019', '\u201A', '\u201B':
				return '\''
			// Fancy double quotes.
			case '\u201C', '\u201D', '\u201E', '\u201F':
				return '"'
			// Non-breaking space and other odd spaces.
			case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007',
				'\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
				return ' '
			}
			return r
		}, strings.TrimSpace(s))
}
