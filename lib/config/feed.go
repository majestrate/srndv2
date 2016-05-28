package config

import (
	"time"
)

// configuration for 1 nntp feed
type FeedConfig struct {
	// feed's policy, filters articles
	Policy *ArticleConfig `json:"policy"`
	// remote server's address
	Addr string `json:"addr"`
	// do we want to periodically pull from this server?
	PullSync bool `json:"pull_enabled"`
	// proxy server config
	Proxy *ProxyConfig `json:"proxy"`
	// nntp username to log in with
	Username string `json:"-"`
	// nntp password to use when logging in
	Password string `json:"-"`
	// do we want to use tls?
	TLS bool `json:"tls"`
	// the name of this feed
	Name string `json:"name"`
	// how often to pull articles from the server
	PullInterval time.Duration `json:"pull_interval"`
}
