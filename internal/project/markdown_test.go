package project

import "testing"

func TestCountWords(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{"empty", "", 0},
		{"plain prose", "The quick brown fox.", 4},
		{"heading and emphasis", "# Chapter One\n\nIt was **dark** and _quiet_.", 7},
		{"list items", "- one\n- two\n- three", 3},
		{"link keeps its label", "See [the appendix](https://example.com/x) for more.", 5},
		{"image keeps its alt text", "![a map](map.png) of the coast", 5},
		{"inline code", "Call `run()` first.", 3},
		{"fenced code keeps the code", "```go\nfunc main() {}\n```", 2},
		{"thematic break is not a word", "one\n\n---\n\ntwo", 2},
		{"punctuation only", "--- *** ===", 0},
		{"numbers count", "In 1984 there were 12 districts.", 6},
		{"footnote reference", "The claim[^1] is old.", 4},
		{"html comment", "one <!-- hidden two --> three", 2},
		{"blockquote", "> a quoted line", 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CountWords(test.markdown); got != test.want {
				t.Errorf("CountWords(%q) = %d, want %d", test.markdown, got, test.want)
			}
		})
	}
}

func TestCountCharactersIgnoresWhitespace(t *testing.T) {
	if got := CountCharacters("ab cd\nef"); got != 6 {
		t.Errorf("CountCharacters = %d, want 6", got)
	}
	// The Markdown markers are not characters the reader sees.
	if got := CountCharacters("**bold**"); got != 4 {
		t.Errorf("CountCharacters of markup = %d, want 4", got)
	}
}

func TestSlugify(t *testing.T) {
	tests := map[string]string{
		"The Long Winter":  "the-long-winter",
		"  spaced  name  ": "spaced-name",
		"A Book: Part 1!":  "a-book-part-1",
		"Ünïcode Näme":     "n-code-n-me",
		"---":              "project",
		"":                 "project",
	}
	for input, want := range tests {
		if got := Slugify(input); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	if got := CleanTitle("   ", "Fallback"); got != "Fallback" {
		t.Errorf("CleanTitle of blank = %q, want Fallback", got)
	}
	if got := CleanTitle("  Chapter  ", "Fallback"); got != "Chapter" {
		t.Errorf("CleanTitle = %q, want Chapter", got)
	}
}

func TestExcerpt(t *testing.T) {
	text := "The first paragraph sets the scene. The second paragraph introduces the character and the problem."
	got := Excerpt(text, "character", 40)
	if !contains(got, "character") {
		t.Errorf("Excerpt(%q) does not contain the query: %q", text, got)
	}
	if len(got) == 0 {
		t.Error("Excerpt returned nothing")
	}

	short := "A short line."
	if got := Excerpt(short, "short", 40); got != short {
		t.Errorf("Excerpt of short text = %q, want the text unchanged", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
