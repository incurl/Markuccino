package project

import "testing"

// TestSearch verifies that the metadata and the bodies are searched.
func TestSearch(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Search")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	draft := findItem(t, mustItems(t, store, project.ID), DraftRootTitle)
	document, err := store.CreateItem(project.ID, draft.ID, KindDocument, "The Lighthouse")
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	if _, err := store.SaveDocument(document.ID, "The keeper climbed the stairs every night.\n"); err != nil {
		t.Fatalf("save: %v", err)
	}

	byTitle, err := store.Search(project.ID, "lighthouse", 10)
	if err != nil {
		t.Fatalf("search the title: %v", err)
	}
	if len(byTitle) != 1 || byTitle[0].Field != "title" {
		t.Errorf("title search = %+v", byTitle)
	}

	byBody, err := store.Search(project.ID, "keeper", 10)
	if err != nil {
		t.Fatalf("search the body: %v", err)
	}
	if len(byBody) != 1 || byBody[0].Field != "body" {
		t.Errorf("body search = %+v", byBody)
	}
	if byBody[0].ItemID != document.ID {
		t.Errorf("hit item = %q, want %q", byBody[0].ItemID, document.ID)
	}

	if hits, err := store.Search(project.ID, "   ", 10); err != nil || len(hits) != 0 {
		t.Errorf("an empty query returned %d hits and %v", len(hits), err)
	}
}

// TestReferenceSheets verifies the character/location/item sheets.
func TestReferenceSheets(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Sheets")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	sheet, err := store.SaveReferenceSheet(ReferenceSheet{
		ProjectID: project.ID,
		Kind:      "character",
		Name:      "Mira",
		Summary:   "The lighthouse keeper",
		Fields:    map[string]any{"age": 34},
	})
	if err != nil {
		t.Fatalf("save sheet: %v", err)
	}
	if sheet.ID == "" || sheet.Fields["age"] != float64(34) {
		t.Errorf("sheet = %+v", sheet)
	}

	sheets, err := store.ListReferenceSheets(project.ID, "character")
	if err != nil {
		t.Fatalf("list sheets: %v", err)
	}
	if len(sheets) != 1 || sheets[0].Name != "Mira" {
		t.Errorf("sheets = %+v", sheets)
	}

	if _, err := store.SaveReferenceSheet(ReferenceSheet{
		ProjectID: project.ID, Kind: "planet", Name: "Mars",
	}); err == nil {
		t.Error("an unknown sheet kind should be rejected")
	}

	if err := store.DeleteReferenceSheet(sheet.ID); err != nil {
		t.Fatalf("delete sheet: %v", err)
	}
	if sheets, err := store.ListReferenceSheets(project.ID, ""); err != nil || len(sheets) != 0 {
		t.Errorf("after delete: %d sheets, %v", len(sheets), err)
	}
}

// TestLayoutAndLastOpenItem verifies the session state round trip.
func TestLayoutAndLastOpenItem(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Session")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	draft := findItem(t, mustItems(t, store, project.ID), DraftRootTitle)

	layout := Layout{
		View:              ViewCorkboard,
		Split:             SplitVertical,
		AIOpen:            false,
		InspectorOpen:     true,
		ExpandedFolderIDs: []string{draft.ID},
	}
	if err := store.SaveLayout(project.ID, layout); err != nil {
		t.Fatalf("save layout: %v", err)
	}
	if err := store.SetLastOpenItem(project.ID, draft.ID); err != nil {
		t.Fatalf("set the open item: %v", err)
	}

	loaded, err := store.GetProject(project.ID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if loaded.Layout.View != ViewCorkboard || loaded.Layout.Split != SplitVertical {
		t.Errorf("layout = %+v", loaded.Layout)
	}
	if loaded.Layout.AIOpen {
		t.Error("the AI panel should be closed")
	}
	if loaded.LastOpenItemID != draft.ID {
		t.Errorf("last open item = %q", loaded.LastOpenItemID)
	}
}

// TestScrivenings verifies that several documents read as one surface and
// that saving them back updates each one.
func TestScrivenings(t *testing.T) {
	store := newTestStore(t)
	project, err := store.CreateProject("Scrivenings")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	draft := findItem(t, mustItems(t, store, project.ID), DraftRootTitle)

	first, err := store.CreateItem(project.ID, draft.ID, KindDocument, "One")
	if err != nil {
		t.Fatalf("create the first document: %v", err)
	}
	second, err := store.CreateItem(project.ID, draft.ID, KindDocument, "Two")
	if err != nil {
		t.Fatalf("create the second document: %v", err)
	}

	scrivenings, err := store.ReadScrivenings([]string{first.ID, second.ID})
	if err != nil {
		t.Fatalf("read the scrivenings: %v", err)
	}
	if len(scrivenings.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(scrivenings.Sections))
	}

	saved, err := store.SaveScrivenings([]Section{
		{ItemID: first.ID, Content: "First section.\n"},
		{ItemID: second.ID, Content: "Second section.\n"},
	})
	if err != nil {
		t.Fatalf("save the scrivenings: %v", err)
	}
	if len(saved.Sections) != 2 || saved.Sections[1].Content != "Second section.\n" {
		t.Errorf("saved = %+v", saved)
	}

	for _, want := range []struct {
		id      string
		content string
	}{
		{first.ID, "First section.\n"},
		{second.ID, "Second section.\n"},
	} {
		document, readErr := store.ReadDocument(want.id)
		if readErr != nil {
			t.Fatalf("read %s: %v", want.id, readErr)
		}
		if document.Content != want.content {
			t.Errorf("document %s = %q, want %q", want.id, document.Content, want.content)
		}
	}
}
