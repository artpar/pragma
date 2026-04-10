package sysprompt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/model"
)

// EnvInfo holds detected environment data.
type EnvInfo struct {
	CWD       string
	Platform  string
	Shell     string
	OSVersion string
	GitRoot   string
	GitBranch string
	IsGitRepo bool
	Model     string
	Date      string
}

// DetectEnv gathers environment information.
func DetectEnv(workDir, modelID string) EnvInfo {
	info := EnvInfo{
		CWD:      workDir,
		Platform: runtime.GOOS,
		Shell:    os.Getenv("SHELL"),
		Model:    modelID,
		Date:     time.Now().Format("2006-01-02"),
	}

	info.OSVersion = osVersion()

	gitRoot := GitRoot(workDir)
	if gitRoot != "" {
		info.IsGitRepo = true
		info.GitRoot = gitRoot
		info.GitBranch = GitBranch(workDir)
	}

	return info
}

// envBlock formats EnvInfo as a SystemBlock.
func envBlock(info EnvInfo) model.SystemBlock {
	var b strings.Builder
	b.WriteString("# Environment\n")
	fmt.Fprintf(&b, "Working directory: %s\n", info.CWD)

	if info.IsGitRepo {
		if info.GitBranch != "" {
			fmt.Fprintf(&b, "Is git repo: yes (branch: %s)\n", info.GitBranch)
		} else {
			b.WriteString("Is git repo: yes\n")
		}
	} else {
		b.WriteString("Is git repo: no\n")
	}

	fmt.Fprintf(&b, "Platform: %s\n", info.Platform)

	if info.Shell != "" {
		fmt.Fprintf(&b, "Shell: %s\n", info.Shell)
	}

	if info.OSVersion != "" {
		fmt.Fprintf(&b, "OS: %s\n", info.OSVersion)
	}

	if info.Model != "" {
		fmt.Fprintf(&b, "Model: %s\n", info.Model)
	}

	fmt.Fprintf(&b, "Date: %s\n", info.Date)

	return model.SystemBlock{Text: b.String(), Cacheable: false}
}

// osVersion returns OS type + release (e.g., "Darwin 24.4.0").
func osVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "uname", "-sr")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
