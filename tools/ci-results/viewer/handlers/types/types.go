package types

type ExecuteQueryRequest struct {
	SQL    string         `json:"sql"`
	Params map[string]any `json:"params"`
}

type QueryColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ExecuteQueryResponse struct {
	Columns    []QueryColumn    `json:"columns"`
	Rows       []map[string]any `json:"rows"`
	RowCount   int              `json:"row_count"`
	DurationMS int64            `json:"duration_ms"`
	Truncated  bool             `json:"truncated"`
}

type SchemaObjectKind string

const (
	SchemaObjectTable SchemaObjectKind = "table"
	SchemaObjectView  SchemaObjectKind = "view"
)

type DBSchemaInfo struct {
	Objects []SchemaObject `json:"objects"`
}

type SchemaObject struct {
	Name    string           `json:"name"`
	Kind    SchemaObjectKind `json:"kind"`
	Columns []SchemaColumn   `json:"columns"`
}

type SchemaColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	PrimaryKey bool   `json:"primaryKey"`
	Nullable   bool   `json:"nullable"`
}
