package version

// Version is a static label for this project.
// Per project policy, set explicitly and not overridden at build time.
const Version = "1.0.8"

func Get() string { return Version }
