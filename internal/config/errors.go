package config

type Source int

const (
	SourceDefaults Source = iota
	SourceGlobal
	SourceProject
	SourceEnv
)

type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

type Error struct {
	Source   Source
	Severity Severity
	Path     string
	Line     int
	Column   int
	Key      string
	Message  string
	Hint     string
}

func (e Error) Error() string {
	return e.Message
}
