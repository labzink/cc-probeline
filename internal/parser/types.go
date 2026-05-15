// Package parser parses Claude Code JSONL transcripts into typed records.
package parser

import (
	"encoding/json"
	"time"
)

// Record is one decoded JSONL line surfaced to consumers.
type Record struct {
	Type, UUID, RequestID, MessageID, ParentUUID string
	Timestamp                                    time.Time

	SessionID, CWD, GitBranch, Version string
	IsSidechain                        bool
	UserType                           string

	Model, ServiceTier, StopReason string
	Usage                          TokenCounts
	Content                        []ContentBlock
}

// TokenCounts holds the per-record usage breakdown.
type TokenCounts struct {
	Input, Output, CacheRead, CacheCreate int
	CacheCreate5m, CacheCreate1h          int
}

// ContentBlock is a single element of message.content (text, tool_use, etc.).
type ContentBlock struct {
	Type, ToolName string
	ToolInput      json.RawMessage
}

// ParseError describes a line-level problem encountered while parsing.
type ParseError struct {
	LineNumber int
	Reason     string
}
