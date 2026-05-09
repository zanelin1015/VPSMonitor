package version

import "runtime"

var (
	Version   = "0.1.0"
	BuildTime = ""
	GitCommit = ""
)

type Info struct {
	Role      string `json:"role"`
	Version   string `json:"version"`
	BuildTime string `json:"build_time,omitempty"`
	GitCommit string `json:"git_commit,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
	Platform  string `json:"platform,omitempty"`
}

func Get(role string) Info {
	return Info{
		Role:      role,
		Version:   Version,
		BuildTime: BuildTime,
		GitCommit: GitCommit,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
}

func String(role string) string {
	info := Get(role)
	if info.GitCommit != "" {
		return "VPSMonitor " + role + " " + info.Version + " (" + info.GitCommit + ")"
	}
	return "VPSMonitor " + role + " " + info.Version
}
