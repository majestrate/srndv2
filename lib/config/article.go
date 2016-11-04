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

func (c *ArticleConfig) AllowGroup(group string) bool {
	// check disallowed groups
	for _, g := range c.DisallowGroups {
		if g == group {
			// disallowed
			return false
		}
	}

	allow := !c.ForceWhitelist

	// check allowed groups
	for _, g := range c.AllowGroups {
		if g == group {
			allow = true
			break
		}
	}
	return allow
}

// allow an article?
func (c *ArticleConfig) Allow(msgid, group string, anon, attachment bool) bool {

	// check attachment policy
	if c.AllowGroup(group) {
		allow := true
		// no anon ?
		if anon && !c.AllowAnon {
			allow = false
		}
		// no attachments ?
		if allow && attachment && !c.AllowAttachments {
			allow = false
		}
		// no anon attachments ?
		if allow && attachment && anon && !c.AllowAnonAttachments {
			allow = false
		}
		return allow
	} else {
		return false
	}
}

var DefaultArticlePolicy = ArticleConfig{
	AllowGroups:          []string{"ctl", "overchan.test"},
	DisallowGroups:       []string{"overchan.cp"},
	ForceWhitelist:       false,
	AllowAnon:            true,
	AllowAttachments:     true,
	AllowAnonAttachments: false,
}
