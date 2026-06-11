package sessionpath

import (
	"path/filepath"
	"github.com/artpar/pragma/internal/observe"
)

const ToolResultsDirName = "tool-results"

func ArtifactDir(sessionsDir, sessionID string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: filepath.Join(sessionsDir, sessionID)")
	return filepath.Join(sessionsDir, sessionID)
}

func ToolResultsDir(sessionsDir, sessionID string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: filepath.Join(ArtifactDir(sessionsDir, sessionID), ToolResultsDirName)")
	return filepath.Join(ArtifactDir(sessionsDir, sessionID), ToolResultsDirName)
}
