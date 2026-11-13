package project

import (
	"fmt"
	"strings"
)

// maxDiffLines bounds the size of a single textual diff.
//
// The diff is computed with a quadratic longest-common-subsequence table,
// which is the right trade-off for a chapter and the wrong one for a
// whole manuscript. Beyond the bound the caller gets a plain explanation
// instead of a frozen window; the History panel still lists the revision.
const maxDiffLines = 2000

// UnifiedDiff renders the change from old to new as a unified diff.
//
// path is the file the diff is for. The headers name the two revisions
// rather than "a/" and "b/", because the UI shows the hashes and the two
// sides are commits, not working-tree states.
func UnifiedDiff(path, fromHash, toHash, old, new string) string {
	from := splitLines(old)
	to := splitLines(new)

	if len(from) > maxDiffLines || len(to) > maxDiffLines {
		return fmt.Sprintf("(the file has %d and %d lines; the diff is limited to %d lines per side)\n",
			len(from), len(to), maxDiffLines)
	}

	ops := diffLines(from, to)
	if len(ops) == 0 {
		return ""
	}
	// The header is only meaningful when at least one line changed.
	changed := false
	for _, op := range ops {
		if op.kind != ' ' {
			changed = true
			break
		}
	}
	if !changed {
		return ""
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "--- %s %s\n", path, shortHash(fromHash))
	fmt.Fprintf(&builder, "+++ %s %s\n", path, shortHash(toHash))
	writeHunks(&builder, ops, from, to)
	return builder.String()
}

// diffOp is one line of the edit script.
type diffOp struct {
	// kind is ' ' for context, '-' for a removed line and '+' for an
	// added line.
	kind byte
	// fromIndex and toIndex are the positions in the two inputs. A
	// removed line has no toIndex; an added line has no fromIndex.
	fromIndex int
	toIndex   int
}

// diffLines computes the edit script between two line slices.
//
// It is a plain LCS table followed by a backtrack. The table holds one
// int per cell, so the bound above keeps it to a few megabytes.
func diffLines(from, to []string) []diffOp {
	n, m := len(from), len(to)

	// lcs[i][j] is the length of the longest common subsequence of
	// from[i:] and to[j:].
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if from[i] == to[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	ops := make([]diffOp, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case from[i] == to[j]:
			ops = append(ops, diffOp{kind: ' ', fromIndex: i, toIndex: j})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{kind: '-', fromIndex: i, toIndex: -1})
			i++
		default:
			ops = append(ops, diffOp{kind: '+', fromIndex: -1, toIndex: j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{kind: '-', fromIndex: i, toIndex: -1})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{kind: '+', fromIndex: -1, toIndex: j})
	}
	return ops
}

// hunkContext is the number of unchanged lines kept around a change.
const hunkContext = 3

// writeHunks groups the edit script into hunks and writes them.
//
// Two changes belong to the same hunk when they are at most twice the
// context distance apart; otherwise the unchanged lines between them are
// collapsed and a new hunk starts.
func writeHunks(builder *strings.Builder, ops []diffOp, from, to []string) {
	changes := make([]int, 0, 8)
	for index, op := range ops {
		if op.kind != ' ' {
			changes = append(changes, index)
		}
	}
	if len(changes) == 0 {
		return
	}

	start := changes[0] - hunkContext
	if start < 0 {
		start = 0
	}
	last := changes[0]

	flush := func(end int) {
		if end > len(ops) {
			end = len(ops)
		}
		if start < end {
			writeHunk(builder, ops[start:end], from, to)
		}
	}

	for _, change := range changes[1:] {
		if change-last > 2*hunkContext+1 {
			flush(last + hunkContext + 1)
			start = change - hunkContext
			if start < 0 {
				start = 0
			}
		}
		last = change
	}
	flush(last + hunkContext + 1)
}

// writeHunk writes one hunk, with its @@ header.
func writeHunk(builder *strings.Builder, ops []diffOp, from, to []string) {
	if len(ops) == 0 {
		return
	}

	fromStart, toStart := -1, -1
	fromCount, toCount := 0, 0
	for _, op := range ops {
		if op.kind != '+' && fromStart < 0 {
			fromStart = op.fromIndex
		}
		if op.kind == ' ' || op.kind == '-' {
			fromCount++
		}
		if op.kind != '-' && toStart < 0 {
			toStart = op.toIndex
		}
		if op.kind == ' ' || op.kind == '+' {
			toCount++
		}
	}
	if fromStart < 0 {
		fromStart = 0
	}
	if toStart < 0 {
		toStart = 0
	}

	fmt.Fprintf(builder, "@@ -%d,%d +%d,%d @@\n",
		fromStart+1, fromCount, toStart+1, toCount)
	for _, op := range ops {
		switch op.kind {
		case ' ':
			builder.WriteString(" ")
			builder.WriteString(from[op.fromIndex])
		case '-':
			builder.WriteString("-")
			builder.WriteString(from[op.fromIndex])
		case '+':
			builder.WriteString("+")
			builder.WriteString(to[op.toIndex])
		}
		builder.WriteString("\n")
	}
}

// splitLines splits text into lines without their terminators. A trailing
// newline does not produce a final empty line, which keeps the diff free
// of a phantom change at the end of every file.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}

// shortHash abbreviates a hash for a diff header.
func shortHash(hash string) string {
	if hash == "" {
		return "(uncommitted)"
	}
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
