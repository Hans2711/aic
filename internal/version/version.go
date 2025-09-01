package version

// Version is set at build time via -ldflags if desired.
var Version = "0.1.0"

func Get() string { return Version }
