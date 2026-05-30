package models

type LinkState string

const (
	Up    LinkState = "Up"
	Down  LinkState = "Down"
	Error LinkState = "Error"
)

type Link struct {
	ID        string
	SourcePID int
	TargetPID int
	MTU       int
	State     LinkState
}

type SetupLinkParams struct {
	SourcePID int
	HostName  string
	Container string
	MTU       int
}
