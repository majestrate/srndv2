package config

type NNTPServerConfig struct {
	// address to bind to
	Bind string
	// name of the nntp server
	Name string
	// default inbound article policy
	Article *ArticleConfig
}
