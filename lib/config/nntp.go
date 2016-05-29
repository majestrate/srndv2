package config

type NNTPServerConfig struct {
	// address to bind to
	Bind string `json:"bind"`
	// name of the nntp server
	Name string `json:"name"`
	// default inbound article policy
	Article *ArticleConfig `json:"policy"`
	// do we allow anonymous NNTP sync?
	AnonNNTP bool `json:"anon-nntp"`
}

var DefaultNNTPConfig = NNTPServerConfig{
	AnonNNTP: false,
	Bind:     "127.0.0.1:1119",
	Name:     "nntp.server.tld",
	Article:  &DefaultArticlePolicy,
}
