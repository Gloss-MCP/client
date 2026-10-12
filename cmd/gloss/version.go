package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Set at build time via -ldflags "-X main.version=... -X main.commit=...
// -X main.date=...". GoReleaser and `make build` both do; a plain
// `go build` or `go install` leaves them empty and versionString falls
// back to the build info the Go toolchain embeds.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// versionString renders the one-line -version output:
//
//	gloss 0.1.0 (commit abc1234, built 2026-10-12T10:00:00Z)
//
// Commit and date are omitted when unknown. A leading "v" on the version
// is dropped so tags (v0.1.0), `git describe` output and GoReleaser's
// already-stripped {{.Version}} all print the same.
func versionString() string {
	v, c, d := version, commit, date
	if c == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v, c, d = fromBuildInfo(v, info)
		}
	}
	return formatVersion(v, c, d)
}

// fromBuildInfo fills in what ldflags did not: the module version when
// installed via `go install module@version`, and the VCS revision and time
// stamped by a `go build` inside a git checkout.
func fromBuildInfo(v string, info *debug.BuildInfo) (version, commit, date string) {
	if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
		case "vcs.time":
			date = s.Value
		}
	}
	return v, commit, date
}

func formatVersion(v, c, d string) string {
	v = strings.TrimPrefix(v, "v")
	if len(c) > 7 {
		c = c[:7]
	}
	switch {
	case c != "" && d != "":
		return fmt.Sprintf("gloss %s (commit %s, built %s)", v, c, d)
	case c != "":
		return fmt.Sprintf("gloss %s (commit %s)", v, c)
	default:
		return "gloss " + v
	}
}
