package config

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Source identifies which level of the config cascade produced the Config.
// Values match concept §5.1 string constants.
type Source string

const (
	// SourceDefaults means no config file was found; built-in defaults are used.
	SourceDefaults Source = "default"
	// SourceGlobal means the global user config file was loaded.
	SourceGlobal Source = "global"
	// SourceProject means a project-local .cc-probeline.toml was loaded.
	SourceProject Source = "project"
	// SourceEnv means the CC_PROBELINE_CONFIG environment variable pointed to a file.
	SourceEnv Source = "env"
)

// Severity classifies how serious an Error is.
type Severity int

const (
	// SeverityError indicates a fatal parse or type error; the file is unusable.
	SeverityError Severity = iota
	// SeverityWarning indicates a non-fatal issue (e.g. unknown field); the
	// file was still loaded successfully.
	SeverityWarning
)

// Error represents a config loading or validation problem.
// It formats as "file:line:col: field: message" with absent segments omitted.
type Error struct {
	// Source is the cascade level that produced this error.
	Source Source

	// Severity classifies the error (SeverityError or SeverityWarning).
	Severity Severity

	// File is the absolute path of the config file that contains the error.
	// Empty if the error is not associated with a specific file.
	File string

	// Line is the 1-based line number in File, or 0 if unknown.
	Line int

	// Column is the 1-based column number in File, or 0 if unknown.
	Column int

	// Field is the TOML key path that caused the error (e.g. "general.no_color").
	// Empty if the error is not associated with a specific field.
	Field string

	// Message is a human-readable description of the error.
	Message string

	// Hint is an optional suggestion to fix the error (may be empty).
	Hint string
}

// Error implements the error interface, formatting as:
//
//	file:line:col: field: message
//
// Segments without meaningful values are omitted.
func (e Error) Error() string {
	var b strings.Builder
	if e.File != "" {
		b.WriteString(e.File)
		if e.Line > 0 {
			fmt.Fprintf(&b, ":%d", e.Line)
			if e.Column > 0 {
				fmt.Fprintf(&b, ":%d", e.Column)
			}
		}
		b.WriteString(": ")
	}
	if e.Field != "" {
		b.WriteString(e.Field)
		b.WriteString(": ")
	}
	b.WriteString(e.Message)
	return b.String()
}

// newParseError wraps a pelletier *toml.DecodeError into an Error.
// It extracts the row, column, and key path from the DecodeError.
// If the DecodeError does not carry positional info, Line and Column remain 0.
func newParseError(path string, decodeErr *toml.DecodeError) Error {
	row, col := decodeErr.Position()
	key := decodeErr.Key()

	field := ""
	if len(key) > 0 {
		field = strings.Join(key, ".")
	}

	return Error{
		Severity: SeverityError,
		File:     path,
		Line:     row,
		Column:   col,
		Field:    field,
		Message:  decodeErr.Error(),
	}
}

// newStrictMissingErrors converts a *toml.StrictMissingError into a slice of
// SeverityWarning errors, one per unknown field.
func newStrictMissingErrors(path string, strictErr *toml.StrictMissingError) []Error {
	out := make([]Error, 0, len(strictErr.Errors))
	for i := range strictErr.Errors {
		de := &strictErr.Errors[i]
		row, col := de.Position()
		key := de.Key()
		field := ""
		if len(key) > 0 {
			field = strings.Join(key, ".")
		}
		out = append(out, Error{
			Severity: SeverityWarning,
			File:     path,
			Line:     row,
			Column:   col,
			Field:    field,
			Message:  "unknown field",
		})
	}
	return out
}
