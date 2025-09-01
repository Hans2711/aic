package version

// Version is a static label for this project.
// Per project policy, this is always "master" and is not overridden at build time.
const Version = "master"

func Get() string { return Version }
