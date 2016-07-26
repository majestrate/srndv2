package config

type DatabaseConfig struct {
	// url or address for database connector
	Addr string `json:"addr"`
	// password to use
	Password string `json:"password"`
	// type of database to use
	Type string `json:"type"`
}

var DefaultDatabaseConfig = DatabaseConfig{
	Type:     "redis",
	Addr:     "127.0.0.1:6379",
	Password: "",
}
