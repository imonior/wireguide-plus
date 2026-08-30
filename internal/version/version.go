// Package version holds the application version.
//
// The single source of truth is the repository-root VERSION file. build/*/Taskfile.yml
// read it and inject the value at build time through
//
//	-ldflags "-X github.com/imonior/wireguide-plus/internal/version.Version=<x.y.z>"
//
// A bare `go build` / `go run` / `go test` (no -ldflags) falls back to the
// dev sentinel below, which also marks the binary as a dev build (the
// auto-update scheduler checks for "-dev" markers via update.IsDevBuild).
package version

// Version is the application version string (e.g. "1.1.1"). Do not edit by
// hand: it is overwritten at build time from the VERSION file.
var Version = "0.0.0-dev"
