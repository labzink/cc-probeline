package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"time"
)

type rawContentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type rawCacheCreation struct {
	Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
}

type rawUsage struct {
	Input        int              `json:"input_tokens"`
	Output       int              `json:"output_tokens"`
	CacheRead    int              `json:"cache_read_input_tokens"`
	CacheCreate  int              `json:"cache_creation_input_tokens"`
	CacheDetails rawCacheCreation `json:"cache_creation"`
}

type rawMessage struct {
	ID          string            `json:"id"`
	Model       string            `json:"model"`
	Usage       *rawUsage         `json:"usage"`
	Content     []rawContentBlock `json:"content"`
	StopReason  string            `json:"stop_reason"`
	ServiceTier string            `json:"service_tier"`
}

type rawLine struct {
	Type            string     `json:"type"`
	UUID            string     `json:"uuid"`
	RequestID       string     `json:"requestId"`
	RequestIDSnake  string     `json:"request_id"`
	ParentUUID      string     `json:"parentUuid"`
	Timestamp       string     `json:"timestamp"`
	SessionID       string     `json:"sessionId"`
	CWD             string     `json:"cwd"`
	GitBranch       string     `json:"gitBranch"`
	Version         string     `json:"version"`
	IsSidechain     bool       `json:"isSidechain"`
	UserType        string     `json:"userType"`
	Message         rawMessage `json:"message"`
}

// ParseLines reads JSONL records from r, decodes them into Record values,
// deduplicates by RequestID/UUID, and returns the slice sorted by Timestamp
// ascending. Line-level errors are accumulated in parseErrors; an err is
// returned only for underlying I/O failures from the scanner.
func ParseLines(r io.Reader) ([]Record, []ParseError, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var records []Record
	var parseErrors []ParseError
	lineNo := 0

	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var raw rawLine
		if err := json.Unmarshal(line, &raw); err != nil {
			parseErrors = append(parseErrors, ParseError{LineNumber: lineNo, Reason: err.Error()})
			continue
		}
		if raw.RequestID == "" {
			raw.RequestID = raw.RequestIDSnake
		}
		if raw.Type != "assistant" {
			continue
		}
		if raw.Message.Usage == nil || isUsageEmpty(raw.Message.Usage) {
			continue
		}

		u := raw.Message.Usage
		rec := Record{
			Type:        raw.Type,
			UUID:        raw.UUID,
			RequestID:   raw.RequestID,
			MessageID:   raw.Message.ID,
			ParentUUID:  raw.ParentUUID,
			SessionID:   raw.SessionID,
			CWD:         raw.CWD,
			GitBranch:   raw.GitBranch,
			Version:     raw.Version,
			IsSidechain: raw.IsSidechain,
			UserType:    raw.UserType,
			Model:       raw.Message.Model,
			ServiceTier: raw.Message.ServiceTier,
			StopReason:  raw.Message.StopReason,
			Usage: TokenCounts{
				Input:         u.Input,
				Output:        u.Output,
				CacheRead:     u.CacheRead,
				CacheCreate:   u.CacheCreate,
				CacheCreate5m: u.CacheDetails.Ephemeral5m,
				CacheCreate1h: u.CacheDetails.Ephemeral1h,
			},
		}

		if raw.Timestamp == "" {
			parseErrors = append(parseErrors, ParseError{LineNumber: lineNo, Reason: "missing timestamp"})
		} else if ts, err := time.Parse(time.RFC3339Nano, raw.Timestamp); err == nil {
			rec.Timestamp = ts.UTC()
		}

		hasSplit := u.CacheDetails.Ephemeral5m != 0 || u.CacheDetails.Ephemeral1h != 0
		if hasSplit && u.CacheCreate != u.CacheDetails.Ephemeral5m+u.CacheDetails.Ephemeral1h {
			parseErrors = append(parseErrors, ParseError{LineNumber: lineNo, Reason: "cache_create_mismatch"})
		}

		if len(raw.Message.Content) > 0 {
			rec.Content = make([]ContentBlock, len(raw.Message.Content))
			for i, c := range raw.Message.Content {
				rec.Content[i] = ContentBlock{
					Type:      c.Type,
					ToolName:  c.Name,
					ToolInput: c.Input,
				}
			}
		}

		if rec.RequestID == "" && rec.MessageID == "" && rec.UUID == "" {
			parseErrors = append(parseErrors, ParseError{LineNumber: lineNo, Reason: "no dedup key"})
		}

		records = append(records, rec)
	}

	if err := sc.Err(); err != nil {
		return records, parseErrors, err
	}

	records = Dedup(records)
	// SliceStable is required: Dedup preserves first-encountered position as a
	// tie-breaker for equal-timestamp records (concept §3 "Deterministic ordering").
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	return records, parseErrors, nil
}

func isUsageEmpty(u *rawUsage) bool {
	return u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheCreate == 0 &&
		u.CacheDetails.Ephemeral5m == 0 && u.CacheDetails.Ephemeral1h == 0
}
