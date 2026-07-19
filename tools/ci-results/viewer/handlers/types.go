package handlers

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
