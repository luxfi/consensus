package quasar

// Config represents quasar protocol configuration
type Config struct {
	QThreshold    int
	QuasarTimeout int
}

// DefaultConfig for quasar protocol
var DefaultConfig = Config{QThreshold: 3, QuasarTimeout: 30}
