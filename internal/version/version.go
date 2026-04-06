package version

// Set at link time via -ldflags -X (see Dockerfile). Defaults support local `go build`.
var (
	Version   = "dev"
	Revision  = "unknown"
	BuildDate = "unknown"
)
