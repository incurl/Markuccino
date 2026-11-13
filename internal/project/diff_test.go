package project

import (
	"strings"
	"testing"
)

func TestUnifiedDiffOfAnEdit(t *testing.T) {
	old := "one\ntwo\nthree\n"
	new := "one\nTWO\nthree\n"

	diff := UnifiedDiff("documents/abc.md", "aaaa1111", "bbbb2222", old, new)
	if diff == "" {
		t.Fatal("the diff is empty")
	}
	for _, want := range []string{"--- documents/abc.md aaaa1111", "+++ documents/abc.md bbbb2222", "-two", "+TWO"} {
		if !strings.Contains(diff, want) {
			t.Errorf("the diff does not contain %q:\n%s", want, diff)
		}
	}
}

func TestUnifiedDiffOfNoChange(t *testing.T) {
	if diff := UnifiedDiff("a.md", "a", "b", "same\n", "same\n"); diff != "" {
		t.Errorf("a diff with no change should be empty, got:\n%s", diff)
	}
}

func TestUnifiedDiffOfAnInsertion(t *testing.T) {
	diff := UnifiedDiff("a.md", "", "b", "one\nthree\n", "one\ntwo\nthree\n")
	if !strings.Contains(diff, "+two") {
		t.Errorf("the diff is missing the inserted line:\n%s", diff)
	}
	if strings.Contains(diff, "-two") {
		t.Errorf("the diff removed a line that was only inserted:\n%s", diff)
	}
}

func TestUnifiedDiffSplitsDistantChanges(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 40; i++ {
		oldLines = append(oldLines, "line")
		newLines = append(newLines, "line")
	}
	newLines[1] = "first change"
	newLines[38] = "second change"

	diff := UnifiedDiff("a.md", "a", "b", strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	if got := strings.Count(diff, "@@ -"); got != 2 {
		t.Errorf("expected 2 hunks, got %d:\n%s", got, diff)
	}
}

func TestUnifiedDiffRefusesAHugeFile(t *testing.T) {
	big := strings.Repeat("line\n", maxDiffLines+1)
	diff := UnifiedDiff("a.md", "a", "b", big, big+"extra\n")
	if !strings.Contains(diff, "limited") {
		t.Errorf("a huge file should be reported, got:\n%s", diff)
	}
}
