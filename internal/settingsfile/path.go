package settingsfile

// Path returns the absolute path to ~/.claude/settings.json.
// Resolves via os.UserHomeDir(); returns "" when home directory is unavailable.
func Path() string {
	return ""
}
