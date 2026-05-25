package config

// Validate runs semantic checks on cfg. Returns []Error (empty == valid).
// Stub: returns nil. GREEN will implement per concept §6.1.
func Validate(cfg *Config) []Error {
	return nil
}

// ApplyRangeFix mutates cfg in place, replacing each field that Validate
// would flag as SeverityError with the default value. Returns the list of
// fixed field paths (for slog debugging).
// Stub: returns nil. GREEN will implement.
func ApplyRangeFix(cfg *Config) []string {
	return nil
}
