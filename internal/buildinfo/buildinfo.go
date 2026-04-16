package buildinfo

// Build-time variables injected via ldflags:
//   -X github.com/artpar/pragma/internal/buildinfo.Version=x.y.z
//   -X github.com/artpar/pragma/internal/buildinfo.Commit=abc1234
//   -X github.com/artpar/pragma/internal/buildinfo.Date=2026-04-12T00:00:00Z
//   -X github.com/artpar/pragma/internal/buildinfo.GoVersion=go1.25.0
var (
	Version   = "dev"
	Commit    = "unknown"
	Date      = "unknown"
	GoVersion = "unknown"
)
