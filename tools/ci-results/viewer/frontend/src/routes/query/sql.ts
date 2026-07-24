import type {
  ColumnSuggestion,
  KeywordSuggestion,
  SchemaInfo,
  SchemaObject,
  SchemaObjectSuggestion,
  SchemaColumn,
  Suggestion
} from '$lib/schema_types';

export type QueryToken = {
  value: string;
  kind: 'plain' | 'keyword' | 'string' | 'number' | 'operator';
};

const keywords: KeywordSuggestion[] = [
  'SELECT',
  'FROM',
  'WHERE',
  'JOIN',
  'ON',
  'GROUP BY',
  'ORDER BY',
  'LIMIT'
].map((keyword) => ({
  kind: 'keyword',
  keyword: keyword as KeywordSuggestion['keyword'],
  label: keyword,
  insertText: `${keyword} `,
  detail: 'SQL keyword'
}));

function matchesPrefix(value: string, prefix: string): boolean {
  return value.toLowerCase().startsWith(prefix.toLowerCase());
}

function objectSuggestion(object: SchemaObject): SchemaObjectSuggestion {
  return {
    kind: object.kind,
    label: object.name,
    insertText: object.name,
    detail: object.kind === 'table' ? 'Table' : 'View',
    object
  };
}

function columnSuggestion(object: SchemaObject, column: SchemaColumn): ColumnSuggestion {
  return {
    kind: 'column',
    label: column.name,
    insertText: column.name,
    detail: `${column.type || 'unknown'} · ${object.name}`,
    objectName: object.name,
    column
  };
}

export function getSuggestions(sqlBeforeCursor: string, schema: SchemaInfo): Suggestion[] {
  const trimmed = sqlBeforeCursor.trimEnd();
  const sourceMatch = trimmed.match(/\b(?:FROM|JOIN)\s+([A-Za-z_][A-Za-z0-9_]*)?$/i);

  if (sourceMatch) {
    const prefix = sourceMatch[1] ?? '';
    const uniqueObjects = new Map(
      schema.objects.map((object) => [object.name.toLowerCase(), object])
    );

    return [...uniqueObjects.values()]
      .filter((object) => matchesPrefix(object.name, prefix))
      .map(objectSuggestion);
  }

  const qualifiedColumnMatch = trimmed.match(/([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z0-9_]*)$/);
  if (qualifiedColumnMatch) {
    const object = schema.objects.find(
      (candidate) => candidate.name.toLowerCase() === qualifiedColumnMatch[1].toLowerCase()
    );
    return object
      ? object.columns
          .filter((column) => matchesPrefix(column.name, qualifiedColumnMatch[2]))
          .map((column) => columnSuggestion(object, column))
      : [];
  }

  const prefix = trimmed.match(/[A-Za-z_][A-Za-z0-9_]*$/)?.[0] ?? '';
  return keywords.filter((suggestion) => matchesPrefix(suggestion.label, prefix));
}

export function tokenizeSql(value: string): QueryToken[] {
  const keywordPattern =
    /^(SELECT|FROM|WHERE|JOIN|ON|GROUP|BY|ORDER|LIMIT|AND|OR|AS|ASC|DESC|NULL|IS|NOT|IN|LIKE)\b/i;
  const tokens: QueryToken[] = [];
  let remaining = value;

  while (remaining) {
    const stringMatch = remaining.match(/^'(?:''|[^'])*'/);
    const keywordMatch = remaining.match(keywordPattern);
    const numberMatch = remaining.match(/^\b\d+(?:\.\d+)?\b/);
    const operatorMatch = remaining.match(/^(?:<=|>=|<>|!=|=|<|>|\*|\+|-|\/)/);
    const match =
      stringMatch ?? keywordMatch ?? numberMatch ?? operatorMatch ?? remaining.match(/^[\s\S]/);
    if (!match) break;

    const kind: QueryToken['kind'] = stringMatch
      ? 'string'
      : keywordMatch
        ? 'keyword'
        : numberMatch
          ? 'number'
          : operatorMatch
            ? 'operator'
            : 'plain';
    const previous = tokens.at(-1);
    if (kind === 'plain' && previous?.kind === 'plain') previous.value += match[0];
    else tokens.push({ value: match[0], kind });
    remaining = remaining.slice(match[0].length);
  }

  return tokens;
}

export function querySource(sql: string): string {
  return sql.match(/\bFROM\s+(?:["`[]?)([\w.]+)(?:["`\]]?)/i)?.[1] ?? 'ad-hoc';
}
