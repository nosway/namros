package version

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Info() map[string]string {
	return map[string]string{
		"version": Version,
		"commit":  Commit,
		"date":    Date,
	}
}
