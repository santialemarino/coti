export interface SchemaTextDouble {
  field: (key: string, values?: Record<string, string | number>) => string;
  shared: (key: string, values?: Record<string, string | number>) => string;
}

export function schemaText(prefixed?: boolean): SchemaTextDouble;
