package main

import (
	"runtime/debug"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		name, version, commit, date, want string
	}{
		{"all fields", "0.1.0", "abc1234def5678", "2026-10-12T10:00:00Z", "gloss 0.1.0 (commit abc1234, built 2026-10-12T10:00:00Z)"},
		{"tag prefix stripped", "v0.1.0", "abc1234", "2026-10-12T10:00:00Z", "gloss 0.1.0 (commit abc1234, built 2026-10-12T10:00:00Z)"},
		{"git describe", "v0.1.0-3-gabc1234-dirty", "abc1234", "", "gloss 0.1.0-3-gabc1234-dirty (commit abc1234)"},
		{"prerelease", "0.1.0-rc1", "abc1234", "2026-10-12T10:00:00Z", "gloss 0.1.0-rc1 (commit abc1234, built 2026-10-12T10:00:00Z)"},
		{"no build info", "dev", "", "", "gloss dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatVersion(tt.version, tt.commit, tt.date); got != tt.want {
				t.Errorf("formatVersion = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFromBuildInfo(t *testing.T) {
	vcs := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abc1234def5678"},
		{Key: "vcs.time", Value: "2026-10-12T10:00:00Z"},
	}
	tests := []struct {
		name        string
		version     string
		info        *debug.BuildInfo
		wantVersion string
		wantCommit  string
	}{
		{"go install at a tag", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "v0.1.0", ""},
		{"local build", "dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs}, "dev", "abc1234def5678"},
		{"ldflags version wins", "0.2.0", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "0.2.0", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, c, _ := fromBuildInfo(tt.version, tt.info)
			if v != tt.wantVersion || c != tt.wantCommit {
				t.Errorf("fromBuildInfo = (%q, %q), want (%q, %q)", v, c, tt.wantVersion, tt.wantCommit)
			}
		})
	}
}
