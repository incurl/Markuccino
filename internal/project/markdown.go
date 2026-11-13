package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// markdownPatterns are the markup fragments the word count drops.
//
// The count is meant to answer "how long does this read", not "how many
// bytes is the file", so the syntax of Markdown should not inflate it.
// The patterns are applied in order: the constructs that contain text
// come first, so their text survives, and the punctuation is stripped
// afterwards.
var markdownPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	// Fenced code blocks: keep the code, drop the fence and the language.
	{regexp.MustCompile("(?m)^\\s*```.*$"), ""},
	// Inline code: keep the content, drop the backticks.
	{regexp.MustCompile("`([^`]*)`"), "$1"},
	// Images: keep the alt text, drop the target.
	{regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`), "$1"},
	// Links: keep the label, drop the target.
	{regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`), "$1"},
	// Footnote references: the note body is a separate block.
	{regexp.MustCompile(`\[\^[^\]]*\]`), ""},
	// HTML comments do not read as text.
	{regexp.MustCompile(`(?s)<!--.*?-->`), ""},
	// ATX heading markers, blockquote markers and list bullets.
	{regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`), ""},
	{regexp.MustCompile(`(?m)^\s{0,3}>\s?`), ""},
	{regexp.MustCompile(`(?m)^\s{0,3}([-*+]|\d+[.)])\s+`), ""},
	// Thematic breaks are punctuation only.
	{regexp.MustCompile(`(?m)^\s{0,3}(?:[-*_]\s*){3,}$`), ""},
	// Emphasis and strikethrough markers.
	{regexp.MustCompile(`(\*\*|__|\*|_|~~)`), ""},
	// Table pipes and alignment rows.
	{regexp.MustCompile(`\|`), " "},
}

// CountWords counts the words a Markdown document reads as.
//
// A word is a run of non-space characters that contains at least one
// letter or digit, so a standalone "---" or "**" is not a word. Numbers
// count, because a chapter that says "1984" has a word in it.
func CountWords(markdown string) int {
	text := plainText(markdown)
	count := 0
	for _, word := range strings.Fields(text) {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				count++
				break
			}
		}
	}
	return count
}

// CountCharacters counts the characters of a Markdown document, ignoring
// whitespace.
//
// Scrivener distinguishes "with spaces" from "without"; the outliner
// shows the second. The editor computes the first in the frontend from
// the text the user sees, where the distinction is visible.
func CountCharacters(markdown string) int {
	text := plainText(markdown)
	count := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	return count
}

// plainText strips the Markdown syntax so the counters see the prose.
func plainText(markdown string) string {
	text := markdown
	for _, rule := range markdownPatterns {
		text = rule.pattern.ReplaceAllString(text, rule.replacement)
	}
	return text
}

// Slugify turns a project name into the identifier used in the workspace
// path and the project list.
//
// Only ASCII letters and digits survive; everything else becomes a single
// dash. A name with no ASCII letters still produces a usable slug, and
// the record id keeps the workspace unique either way.
func Slugify(name string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		slug = "project"
	}
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return slug
}

// CleanTitle trims a title and falls back to a default, so a record never
// stores a blank title.
func CleanTitle(title, fallback string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return fallback
	}
	if len(title) > 300 {
		title = strings.TrimSpace(title[:300])
	}
	return title
}

// Excerpt returns a short window of text around the first occurrence of
// query, with ellipses where it was cut.
//
// It is what the search results show: enough context to recognise the
// match without rendering the whole document.
func Excerpt(text, query string, width int) string {
	text = strings.Join(strings.Fields(text), " ")
	if width <= 0 {
		width = 120
	}
	if len(text) <= width {
		return text
	}

	index := -1
	if query != "" {
		index = strings.Index(strings.ToLower(text), strings.ToLower(query))
	}
	if index < 0 {
		return text[:width] + "…"
	}

	start := index - width/3
	if start < 0 {
		start = 0
	}
	end := start + width
	if end > len(text) {
		end = len(text)
		start = end - width
		if start < 0 {
			start = 0
		}
	}

	excerpt := text[start:end]
	if start > 0 {
		excerpt = "…" + excerpt
	}
	if end < len(text) {
		excerpt += "…"
	}
	return excerpt
}

// writeFileAtomic writes content to path through a temporary file.
//
// A crash in the middle of a save must not leave a half-written chapter.
// The rename is atomic on the same filesystem, which the temporary file
// guarantees by living in the same directory.
func writeFileAtomic(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, ".write-*")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	tempName := temp.Name()
	defer func() {
		// The rename removes it on success; on failure it is the one
		// remaining piece of evidence.
		_ = os.Remove(tempName)
	}()

	if _, err := temp.WriteString(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write %s: %w", tempName, err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set the mode of %s: %w", tempName, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tempName, err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
