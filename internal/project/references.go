package project

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// The valid reference sheet kinds.
var referenceKinds = map[string]bool{
	"character": true,
	"location":  true,
	"item":      true,
}

// ListReferenceSheets returns the sheets of a project, optionally
// filtered by kind.
func (s *Store) ListReferenceSheets(projectID, kind string) ([]ReferenceSheet, error) {
	filter := "project = {:project}"
	params := dbx.Params{"project": projectID}
	if kind != "" {
		if !referenceKinds[kind] {
			return nil, fmt.Errorf("project: unknown reference sheet kind %q", kind)
		}
		filter += " && kind = {:kind}"
		params["kind"] = kind
	}
	records, err := s.app.FindRecordsByFilter(CollectionReferenceSheets, filter, "kind,name", 0, 0, params)
	if err != nil {
		return nil, fmt.Errorf("list the reference sheets: %w", err)
	}
	sheets := make([]ReferenceSheet, 0, len(records))
	for _, record := range records {
		sheets = append(sheets, toReferenceSheet(record))
	}
	return sheets, nil
}

// SaveReferenceSheet creates or updates a sheet.
//
// An empty ID creates it. A sheet is linked to documents by record id
// rather than by relation, so a permanently deleted chapter leaves the
// sheet in place with one fewer link.
func (s *Store) SaveReferenceSheet(sheet ReferenceSheet) (ReferenceSheet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !referenceKinds[sheet.Kind] {
		return ReferenceSheet{}, fmt.Errorf("project: unknown reference sheet kind %q", sheet.Kind)
	}
	sheet.Name = CleanTitle(sheet.Name, "Untitled Sheet")
	if sheet.Fields == nil {
		sheet.Fields = map[string]any{}
	}
	if sheet.DocumentIDs == nil {
		sheet.DocumentIDs = []string{}
	}

	collection, err := s.app.FindCollectionByNameOrId(CollectionReferenceSheets)
	if err != nil {
		return ReferenceSheet{}, fmt.Errorf("find the reference sheets collection: %w", err)
	}

	var record *core.Record
	if sheet.ID != "" {
		record, err = s.app.FindRecordById(CollectionReferenceSheets, sheet.ID)
		if err != nil {
			return ReferenceSheet{}, fmt.Errorf("find the reference sheet %q: %w", sheet.ID, err)
		}
	} else {
		if _, err := s.projectRecord(sheet.ProjectID); err != nil {
			return ReferenceSheet{}, err
		}
		record = core.NewRecord(collection)
		record.Set("project", sheet.ProjectID)
	}

	record.Set("kind", sheet.Kind)
	record.Set("name", sheet.Name)
	record.Set("summary", sheet.Summary)
	record.Set("notes", sheet.Notes)
	record.Set("fields", sheet.Fields)
	record.Set("document_ids", sheet.DocumentIDs)
	if err := s.app.Save(record); err != nil {
		return ReferenceSheet{}, fmt.Errorf("save the reference sheet: %w", err)
	}

	s.audit(record.GetString("project"), "reference", "save", ActorUser, "reference_sheet", record.Id,
		map[string]any{"kind": sheet.Kind, "name": sheet.Name})
	return toReferenceSheet(record), nil
}

// DeleteReferenceSheet removes a sheet.
func (s *Store) DeleteReferenceSheet(sheetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.app.FindRecordById(CollectionReferenceSheets, sheetID)
	if err != nil {
		return fmt.Errorf("find the reference sheet %q: %w", sheetID, err)
	}
	projectID := record.GetString("project")
	name := record.GetString("name")
	if err := s.app.Delete(record); err != nil {
		return fmt.Errorf("delete the reference sheet: %w", err)
	}
	s.audit(projectID, "reference", "delete", ActorUser, "reference_sheet", sheetID,
		map[string]any{"name": name})
	return nil
}

// toReferenceSheet converts a record into the frontend shape.
func toReferenceSheet(record *core.Record) ReferenceSheet {
	return ReferenceSheet{
		ID:          record.Id,
		ProjectID:   record.GetString("project"),
		Kind:        record.GetString("kind"),
		Name:        record.GetString("name"),
		Summary:     record.GetString("summary"),
		Notes:       record.GetString("notes"),
		Fields:      decodeMap(record, "fields"),
		DocumentIDs: decodeStrings(record, "document_ids"),
		Created:     formatTime(record, "created"),
		Updated:     formatTime(record, "updated"),
	}
}
