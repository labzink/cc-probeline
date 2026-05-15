package parser

// Dedup removes records sharing a dedup key, keeping the one with the smallest
// Timestamp. Key = RequestID if non-empty, else UUID. Records with both empty
// pass through unchanged. The winner occupies the position of the first
// occurrence of the key in the input slice (stable). Ties on Timestamp are
// resolved by keeping the first encountered record.
func Dedup(records []Record) []Record {
	if len(records) == 0 {
		return nil
	}

	seen := make(map[string]int, len(records))
	result := make([]Record, 0, len(records))

	for _, r := range records {
		key := r.RequestID
		if key == "" {
			key = r.UUID
		}
		if key == "" {
			result = append(result, r)
			continue
		}
		idx, ok := seen[key]
		if !ok {
			seen[key] = len(result)
			result = append(result, r)
			continue
		}
		if r.Timestamp.Before(result[idx].Timestamp) {
			result[idx] = r
		}
	}
	return result
}
