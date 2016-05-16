package nntp

import (
	"encoding/json"
)

//
// a policy that governs whether we federate an article via a feed
//
type FeedPolicy struct {
	// whitelist list for newsgroups to always allow
	Whitelist []Newsgroup
	// list of blacklist regexps
	Blacklist []string
	// are anon posts of any kind allowed?
	AllowAnonPosts bool
	// are anon posts with attachments allowed?
	AllowAnonAttachments bool
	// are any attachments allowed?
	AllowAttachments bool
	// do we require Proof Of Work for untrusted connections?
	UntrustedRequiresPoW bool
}

// marshal feed policy to json
// {
//   "white" : [ list, of, whitelisted, newsgroups ],
//   "black" : [ list, of, newsgroup, blacklist, regexp ],
//   "anon" : bool,
//   "anon-attachments" : bool,
//   "attachments" : bool,
//   "pow": bool
// }
func (p *FeedPolicy) MarshalJSON() ([]byte, error) {
	j := make(map[string]interface{})
	j["white"] = p.Whitelist
	j["black"] = p.Blacklist
	j["anon"] = p.AllowAnonPosts
	j["anon-attachments"] = p.AllowAnonAttachments
	j["attachments"] = p.AllowAttachments
	j["pow"] = p.UntrustedRequiresPoW
	return json.Marshal(j)
}

// default feed policy to be used if not configured explicitly
var DefaultFeedPolicy = &FeedPolicy{
	Whitelist:            []Newsgroup{Newsgroup("ctl"), Newsgroup("overchan.test")},
	Blacklist:            []string{`!^overchan\.`},
	AllowAnonPosts:       true,
	AllowAnonAttachments: false,
	UntrustedRequiresPoW: true,
	AllowAttachments:     true,
}
