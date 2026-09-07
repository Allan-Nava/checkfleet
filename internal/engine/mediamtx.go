package engine

// MediaMTXConfig configures the mediamtx streaming-server check (CF-21).
//
// mediamtx is where a live stream actually lives: the paths it serves, who is
// publishing into them and who is reading out. None of that is visible from the
// outside — an `ingest` probe says a streamer *can* connect and a `stream` probe
// says a manifest is being served, but neither can say "the path exists, someone
// is publishing to it, and bytes are moving".
type MediaMTXConfig struct {
	Targets []MediaMTXTarget `yaml:"targets"`
	// Client certificate for mTLS (CF-183); empty leaves the handshake unchanged.
	ClientTLS ClientTLS `yaml:",inline"`
}

// MediaMTXTarget is one mediamtx control API.
type MediaMTXTarget struct {
	Name string `yaml:"name"`
	// URL of the control API, scheme+host[:port], e.g. http://mtx-01:9997.
	// No trailing slash; the v3 API paths are appended.
	URL string `yaml:"url"`
	// HTTP basic auth when the API is protected. The password is read from the
	// named env var, never written in the config.
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"`
	// Skip TLS verification (self-signed API certificate).
	Insecure bool `yaml:"insecure_skip_verify"`

	// ExpectPaths are the paths that must exist and be ready. A path that is
	// configured in mediamtx but has no publisher does not appear as ready, and
	// that is exactly the outage this check is for: the encoder died and every
	// downstream probe still passes because the manifest is cached.
	ExpectPaths []string `yaml:"expect_paths"`
	// WarnNoReaders WARNs on a ready path nobody is reading. Off by default: a
	// stream with no viewers at 04:00 is not a fault, it is the middle of the
	// night. Turn it on for a path that should always have a consumer.
	WarnNoReaders bool `yaml:"warn_no_readers"`
	// MaxLatencyMS WARNs when the API is slower than this (0 disables).
	MaxLatencyMS int `yaml:"max_latency_ms"`
}
