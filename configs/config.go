package configs

import (
	"fmt"
	"net/url"
)

//nolint:revive
type Config struct {
	Telemetry struct {
		Logs struct {
			Level  string
			Format string
		}
		Metrics struct {
			Enabled     bool
			BindAddress string
		}
		Health struct {
			Enabled     bool
			BindAddress string
		}
	}
	Pfsense struct {
		URL      URL
		Username string
		Password string
		Insecure bool
	}
	DryRun bool
}

type URL url.URL

func (d *URL) UnmarshalText(text []byte) error {
	parsed, err := url.Parse(string(text))
	if err != nil {
		return fmt.Errorf("failed to parse url; %w", err)
	}
	*d = URL(*parsed)
	return nil
}
