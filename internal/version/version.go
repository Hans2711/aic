package version

import releaseinfo "github.com/diesi/aic"

// Version is a static label for this project.
// Per project policy, set explicitly and not overridden at build time.
var Version = releaseinfo.Version()

func Get() string { return Version }
