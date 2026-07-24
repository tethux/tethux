export type SchemaObjectKind = 'table' | 'view';

export interface SchemaInfo {
  objects: SchemaObject[];
}

export interface SchemaObject {
  name: string;
  kind: SchemaObjectKind;
  columns: SchemaColumn[];
}

export interface SchemaColumn {
  name: string;
  type: string;
  primaryKey: boolean;
  nullable: boolean;
}

export type Suggestion =
  KeywordSuggestion | SchemaObjectSuggestion | ColumnSuggestion | FunctionSuggestion;

interface SuggestionBase {
  label: string;
  insertText: string;
  detail?: string;
}

export interface KeywordSuggestion extends SuggestionBase {
  kind: 'keyword';
  keyword: 'SELECT' | 'FROM' | 'WHERE' | 'JOIN' | 'ON' | 'GROUP BY' | 'ORDER BY' | 'LIMIT';
}

export interface SchemaObjectSuggestion extends SuggestionBase {
  kind: 'table' | 'view';
  object: SchemaObject;
}

export interface ColumnSuggestion extends SuggestionBase {
  kind: 'column';
  column: SchemaColumn;
  objectName: string;
}

export interface FunctionSuggestion extends SuggestionBase {
  kind: 'function';
  functionName: string;
}
