package dynamips

// PlatformConfig selects a router family and its hardware settings.
// Empty Chassis and nil MAC preserve native defaults. C7200 may only be
// supplied when Name is "c7200".
type PlatformConfig struct {
	Name    string
	Chassis string
	MAC     *[6]byte
	C7200   *C7200Config
}

// C7200Config contains settings specific to C7200 routers.
// Empty fields preserve native defaults.
type C7200Config struct {
	NPE      string
	Midplane string
}
