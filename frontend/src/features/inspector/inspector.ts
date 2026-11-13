/**
 * Pure inspector helpers: the keyword list and the sheet defaults.
 */

import type { BinderItem, ReferenceSheet } from '../../bindings';

/** parseKeywords splits a comma-separated list into trimmed keywords. */
export function parseKeywords(input: string): string[] {
  return input
    .split(',')
    .map((keyword) => keyword.trim())
    .filter((keyword) => keyword.length > 0);
}

/** keywordsToInput renders the list for the form. */
export function keywordsToInput(keywords: string[]): string {
  return keywords.join(', ');
}

/** sheetKinds is the set of reference sheet kinds the backend accepts. */
export const sheetKinds = ['character', 'location', 'item'] as const;

/** SheetKind is one of the reference sheet kinds. */
export type SheetKind = (typeof sheetKinds)[number];

/**
 * newSheet builds an empty sheet linked to the current document.
 *
 * A sheet started from the inspector is linked to the document the
 * author is looking at, because that is where the character or place
 * appeared.
 */
export function newSheet(projectId: string, item: BinderItem | null, kind: SheetKind = 'character'): ReferenceSheet {
  return {
    id: '',
    project_id: projectId,
    kind,
    name: item ? item.title : 'Untitled Sheet',
    summary: '',
    notes: '',
    fields: {},
    document_ids: item && item.kind === 'document' ? [item.id] : [],
    created: '',
    updated: '',
  };
}

/** linkDocument adds a document id to a sheet, without duplicates. */
export function linkDocument(sheet: ReferenceSheet, itemId: string): ReferenceSheet {
  if (!itemId || sheet.document_ids.includes(itemId)) {
    return sheet;
  }
  return { ...sheet, document_ids: [...sheet.document_ids, itemId] };
}

/** metaFromItem builds the inspector's editable view of an item. */
export function metaFromItem(item: BinderItem): {
  title: string;
  synopsis: string;
  notes: string;
  keywords: string[];
  label: string;
  status: string;
  include_in_compile: boolean;
  custom_meta: Record<string, unknown>;
} {
  return {
    title: item.title,
    synopsis: item.synopsis,
    notes: item.notes,
    keywords: item.keywords,
    label: item.label,
    status: item.status,
    include_in_compile: item.include_in_compile,
    custom_meta: item.custom_meta,
  };
}

/**
 * parseFields reads the structured sheet fields from a "key = value" per
 * line text.
 *
 * It keeps the values as strings, which is what a reference sheet's
 * free-form fields are. A line without a key or without "=" is ignored,
 * so a stray blank line never creates a field called "".
 */
export function parseFields(input: string): Record<string, unknown> {
  const fields: Record<string, unknown> = {};
  for (const line of input.split('\n')) {
    const index = line.indexOf('=');
    if (index < 1) {
      continue;
    }
    const key = line.slice(0, index).trim();
    if (!key) {
      continue;
    }
    fields[key] = line.slice(index + 1).trim();
  }
  return fields;
}

/** fieldsToText renders the structured fields for the form. */
export function fieldsToText(fields: Record<string, unknown>): string {
  return Object.entries(fields)
    .map(([key, value]) => `${key} = ${String(value)}`)
    .join('\n');
}
