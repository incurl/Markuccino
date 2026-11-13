import { describe, expect, it } from 'vitest';

import type { BinderItem, ReferenceSheet } from '../../bindings';
import { fieldsToText, keywordsToInput, linkDocument, newSheet, parseFields, parseKeywords } from './inspector';

describe('parseKeywords', () => {
  it('splits, trims and drops the empty entries', () => {
    expect(parseKeywords(' rain, night ,, storm ')).toEqual(['rain', 'night', 'storm']);
  });

  it('returns an empty list for an empty input', () => {
    expect(parseKeywords('')).toEqual([]);
    expect(parseKeywords(' , , ')).toEqual([]);
  });
});

describe('keywordsToInput', () => {
  it('joins the list for the form', () => {
    expect(keywordsToInput(['a', 'b'])).toBe('a, b');
    expect(keywordsToInput([])).toBe('');
  });
});

const document = {
  id: 'doc1',
  kind: 'document',
  title: 'The Lighthouse',
} as BinderItem;

describe('newSheet', () => {
  it('links a sheet to the open document', () => {
    const sheet = newSheet('p1', document);
    expect(sheet.project_id).toBe('p1');
    expect(sheet.kind).toBe('character');
    expect(sheet.name).toBe('The Lighthouse');
    expect(sheet.document_ids).toEqual(['doc1']);
  });

  it('does not link a folder', () => {
    const sheet = newSheet('p1', { id: 'f1', kind: 'folder', title: 'Part' } as BinderItem, 'location');
    expect(sheet.kind).toBe('location');
    expect(sheet.document_ids).toEqual([]);
  });
});

describe('linkDocument', () => {
  it('adds a document once', () => {
    const sheet = { document_ids: ['a'] } as ReferenceSheet;
    expect(linkDocument(sheet, 'b').document_ids).toEqual(['a', 'b']);
    expect(linkDocument(sheet, 'a')).toBe(sheet);
  });
});

describe('parseFields', () => {
  it('reads one field per line', () => {
    expect(parseFields('age = 34\nrole = keeper\n\nbroken\n= no key')).toEqual({
      age: '34',
      role: 'keeper',
    });
  });

  it('keeps an equals sign in the value', () => {
    expect(parseFields('note = a = b')).toEqual({ note: 'a = b' });
  });
});

describe('fieldsToText', () => {
  it('renders the fields for the form', () => {
    expect(fieldsToText({ age: 34, role: 'keeper' })).toBe('age = 34\nrole = keeper');
  });
});
