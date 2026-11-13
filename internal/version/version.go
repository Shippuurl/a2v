// Package version carries the build-time version string.
package version

// Version is stamped by the build: -ldflags "-X .../version.Version=v1.0.0".
var Version = "dev"

// Base is the public distribution root.
const Base = "https://tool.kervein.club/a2v"
