package config

// configration for local article policies
type ArticleConfig struct {
	// explicitly allow these newsgroups
	AllowGroups []string
	// explicitly disallow these newsgroups
	DisallowGroups []string
	// only allow explicitly allowed groups
	ForceWhitelist bool
	// allow anonymous posts?
	AllowAnon bool
	// allow attachments?
	AllowAttachments bool
	// allow anonymous attachments?
	AllowAnonAttachments bool
}
