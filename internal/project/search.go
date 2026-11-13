package project

import (
	"strings"
)

// Search looks for a query in the titles, synopses, notes and bodies of a
// project.
//
// The bodies live on disk, so the search reads them: PocketBase indexes
// the metadata, not the Markdown. That is the right trade-off for a
// desktop project of a few hundred chapters and it keeps the file the
// source of truth for the text.
func (s *Store) Search(projectID, query string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []SearchHit{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	needle := strings.ToLower(query)

	records, err := s.itemRecords(projectID, "trashed = false", nil)
	if err != nil {
		return nil, err
	}

	hits := make([]SearchHit, 0, limit)
	for _, record := range records {
		item := toItem(record)
		if strings.Contains(strings.ToLower(item.Title), needle) {
			hits = append(hits, SearchHit{
				ItemID: item.ID, Title: item.Title, Field: "title",
				Excerpt: Excerpt(item.Synopsis, query, 120), Bucket: item.Bucket,
			})
		} else if strings.Contains(strings.ToLower(item.Synopsis), needle) {
			hits = append(hits, SearchHit{
				ItemID: item.ID, Title: item.Title, Field: "synopsis",
				Excerpt: Excerpt(item.Synopsis, query, 140), Bucket: item.Bucket,
			})
		} else if strings.Contains(strings.ToLower(item.Notes), needle) {
			hits = append(hits, SearchHit{
				ItemID: item.ID, Title: item.Title, Field: "notes",
				Excerpt: Excerpt(item.Notes, query, 140), Bucket: item.Bucket,
			})
		} else if item.Kind == KindDocument {
			content, readErr := s.readItemFile(record)
			if readErr != nil {
				continue
			}
			if strings.Contains(strings.ToLower(content), needle) {
				hits = append(hits, SearchHit{
					ItemID: item.ID, Title: item.Title, Field: "body",
					Excerpt: Excerpt(content, query, 160), Bucket: item.Bucket,
				})
			}
		}
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}
