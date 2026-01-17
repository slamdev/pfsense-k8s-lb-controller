package testdata

//nolint:revive
type Config struct {
	Config struct {
		Active struct {
			Profiles string
		}
	}
	App struct {
		AutoStart bool
	}
}

var Cfg Config
