package config

// configration for local article policies
type ArticleConfig struct {
	// explicitly allow these newsgroups
	AllowGroups []string `json:"whitelist"`
	// explicitly disallow these newsgroups
	DisallowGroups []string `json:"blacklist"`
	// only allow explicitly allowed groups
	ForceWhitelist bool `json:"force-whitelist"`
	// allow anonymous posts?
	AllowAnon bool `json:"anon"`
	// allow attachments?
	AllowAttachments bool `json:"attachments"`
	// allow anonymous attachments?
	AllowAnonAttachments bool `json:"anon-attachments"`
}

var DefaultArticlePolicy = ArticleConfig{
	AllowGroups:          []string{"ctl", "overchan.test"},
	DisallowGroups:       []string{"overchan.cp"},
	ForceWhitelist:       false,
	AllowAnon:            true,
	AllowAttachments:     true,
	AllowAnonAttachments: false,
}
