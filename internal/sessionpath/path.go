package sessionpath

import "path/filepath"

const ToolResultsDirName = "tool-results"

func ArtifactDir(sessionsDir, sessionID string) string {
	return filepath.Join(sessionsDir, sessionID)
}

func ToolResultsDir(sessionsDir, sessionID string) string {
	return filepath.Join(ArtifactDir(sessionsDir, sessionID), ToolResultsDirName)
}
