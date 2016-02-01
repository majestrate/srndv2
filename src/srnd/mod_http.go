//
// mod_http.go
//
// http mod panel
//

package srnd

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/sessions"
	"github.com/majestrate/nacl"
	"io"
	"log"
	"net/http"
	"strings"
)

type httpModUI struct {
	regenAll         func()
	regen            func(ArticleEntry)
	regenGroup       func(string)
	delete           func(string)
	deleteBoardPages func(string)
	modMessageChan   chan NNTPMessage
	database         Database
	articles         ArticleStore
	store            *sessions.CookieStore
	prefix           string
	mod_prefix       string
}

func createHttpModUI(frontend *httpFrontend) httpModUI {
	return httpModUI{frontend.regenAll, frontend.Regen, frontend.regenerateBoard, frontend.deleteThreadMarkup, frontend.deleteBoardMarkup, make(chan NNTPMessage), frontend.daemon.database, frontend.daemon.store, frontend.store, frontend.prefix, frontend.prefix + "mod/"}

}

func extractGroup(param map[string]interface{}) string {
	return extractParam(param, "newsgroup")
}

func extractParam(param map[string]interface{}, k string) string {
	v, ok := param[k]
	if ok {
		return v.(string)
	}
	return ""
}

func (self httpModUI) getAdminFunc(funcname string) AdminFunc {
	if funcname == "template.reload" {
		return func(param map[string]interface{}) (string, error) {
			tname, ok := param["template"]
			if ok {
				t := ""
				switch tname.(type) {
				case string:
					t = tname.(string)
				default:
					return "failed to reload templates", errors.New("invalid parameters")
				}
				template.reloadTemplate(t)
				return "reloaded " + t, nil
			}
			template.reloadAllTemplates()
			return "reloaded all templates", nil
		}
	} else if funcname == "frontend.regen" {
		return func(param map[string]interface{}) (string, error) {
			newsgroup := extractGroup(param)
			if len(newsgroup) > 0 {
				if self.database.HasNewsgroup(newsgroup) {
					go self.regenGroup(newsgroup)
				} else {
					return "failed to regen group", errors.New("no such board")
				}
			} else {
				go self.regenAll()
			}
			return "started regeneration", nil
		}
	} else if funcname == "thumbnail.regen" {
		return func(param map[string]interface{}) (string, error) {
			threads, ok := param["threads"]
			t := 1
			if ok {
				switch threads.(type) {
				case int64:
					t = int(threads.(int64))
					if t <= 0 {
						return "failed to regen thumbnails", errors.New("invalid number of threads")
					}
				default:
					return "failed to regen thumbnails", errors.New("invalid parameters")
				}
			}
			log.Println("regenerating all thumbnails with", t, "threads")
			go reThumbnail(t, self.articles)
			return fmt.Sprintf("started rethumbnailing with %d threads", t), nil
		}
	} else if funcname == "frontend.add" {
		return func(param map[string]interface{}) (string, error) {
			newsgroup := extractGroup(param)
			if len(newsgroup) > 0 && newsgroupValidFormat(newsgroup) && strings.HasPrefix(newsgroup, "overchan.") && newsgroup != "overchan." {
				if self.database.HasNewsgroup(newsgroup) {
					// we already have this newsgroup
					return "already have that newsgroup", nil
				} else {
					// we dont got this newsgroup
					log.Println("adding newsgroup", newsgroup)
					self.database.RegisterNewsgroup(newsgroup)
					return "added " + newsgroup, nil
				}
			}
			return "bad newsgroup", errors.New("invalid newsgroup name: " + newsgroup)
		}
	} else if funcname == "frontend.ban" {
		return func(param map[string]interface{}) (string, error) {
			newsgroup := extractGroup(param)
			if len(newsgroup) > 0 {
				log.Println("banning", newsgroup)
				// check ban
				banned, err := self.database.NewsgroupBanned(newsgroup)
				if banned {
					// already banned
					return "cannot ban newsgroup", errors.New("already banned " + newsgroup)
				} else if err == nil {
					// do the ban here
					err = self.database.BanNewsgroup(newsgroup)
					// check for error
					if err == nil {
						// all gud
						return "banned " + newsgroup, nil
					} else {
						// error while banning
						return "error banning newsgroup", err
					}
				} else {
					// error checking ban
					return "cannot check ban", err
				}
			} else {
				// bad parameters
				return "cannot ban newsgroup", errors.New("invalid parameters")
			}
		}
	} else if funcname == "frontend.unban" {
		return func(param map[string]interface{}) (string, error) {
			newsgroup := extractGroup(param)
			if len(newsgroup) > 0 {
				log.Println("unbanning", newsgroup)
				err := self.database.UnbanNewsgroup(newsgroup)
				if err == nil {
					return "unbanned " + newsgroup, nil
				} else {
					return "couldn't unban " + newsgroup, err
				}
			} else {
				return "cannot unban", errors.New("invalid paramters")
			}
		}
	} else if funcname == "frontend.nuke" {
		return func(param map[string]interface{}) (string, error) {
			newsgroup := extractGroup(param)
			if len(newsgroup) > 0 {
				log.Println("nuking", newsgroup)
				// get every thread we have in this group
				for _, entry := range self.database.GetLastBumpedThreads(newsgroup, 10000) {
					// delete their thread page
					self.delete(entry.MessageID())
				}
				// delete every board page
				self.deleteBoardPages(newsgroup)
				go self.database.NukeNewsgroup(newsgroup, self.articles)
				return "nuke started", nil
			} else {
				return "cannot nuke", errors.New("invalid parameters")
			}
		}
	} else if funcname == "pubkey.add" {
		return func(param map[string]interface{}) (string, error) {
			pubkey := extractParam(param, "pubkey")
			log.Println("pubkey.add", pubkey)
			if self.database.CheckModPubkeyGlobal(pubkey) {
				return "already added", nil
			} else {
				err := self.database.MarkModPubkeyGlobal(pubkey)
				if err == nil {
					return "added", nil
				} else {
					return "error", err
				}
			}
		}
	} else if funcname == "pubkey.del" {
		return func(param map[string]interface{}) (string, error) {
			pubkey := extractParam(param, "pubkey")
			log.Println("pubkey.del", pubkey)
			if self.database.CheckModPubkeyGlobal(pubkey) {
				err := self.database.UnMarkModPubkeyGlobal(pubkey)
				if err == nil {
					return "removed", nil
				} else {
					return "error", err
				}
			} else {
				return "key not trusted", nil
			}
		}

	} else if funcname == "nntp.login.del" {
		return func(param map[string]interface{}) (string, error) {
			username := extractParam(param, "username")
			if len(username) > 0 {
				exists, err := self.database.CheckNNTPUserExists(username)
				if exists {
					err = self.database.RemoveNNTPLogin(username)
					if err == nil {
						return "removed user", nil
					}
					return "", nil
				} else if err == nil {
					return "no such user", nil
				} else {
					return "", err
				}
			} else {
				return "no such user", nil
			}
		}
	} else if funcname == "nntp.login.add" {
		return func(param map[string]interface{}) (string, error) {
			username := extractParam(param, "username")
			passwd := extractParam(param, "passwd")
			if len(username) > 0 && len(passwd) > 0 {
				log.Println("nntp.login.add", username)
				// check if users is there
				exists, err := self.database.CheckNNTPUserExists(username)
				if exists {
					// user is already there
					return "user already exists", nil
				} else if err == nil {
					// now add the user
					err = self.database.AddNNTPLogin(username, passwd)
					// success adding?
					if err == nil {
						// yeh
						return "added user", nil
					}
					// nah
					return "", err
				} else {
					// error happened
					return "", err
				}
			} else {
				return "invalid username or password format", nil
			}
		}
	}
	return nil
}

// handle an admin action
func (self httpModUI) HandleAdminCommand(wr http.ResponseWriter, r *http.Request) {
	self.asAuthed(func(url string) {
		action := strings.Split(url, "/admin/")[1]
		log.Println("try admin action", action)
		f := self.getAdminFunc(action)
		if f == nil {
			wr.WriteHeader(404)
		} else {
			var msg string
			var err error
			req := make(map[string]interface{})
			if r.Method == "POST" {
				dec := json.NewDecoder(r.Body)
				err = dec.Decode(&req)
			}
			if err == nil {
				msg, err = f(req)
			}
			resp := make(map[string]interface{})
			if err == nil {
				resp["error"] = nil
			} else {
				resp["error"] = err.Error()
			}
			resp["result"] = msg
			enc := json.NewEncoder(wr)
			enc.Encode(resp)
		}

	}, wr, r)
}

// TODO: check for different levels of permissions
func (self httpModUI) CheckKey(privkey string) (bool, error) {
	privkey_bytes, err := hex.DecodeString(privkey)
	if err == nil {
		kp := nacl.LoadSignKey(privkey_bytes)
		if kp != nil {
			defer kp.Free()
			pubkey := hex.EncodeToString(kp.Public())
			if self.database.CheckModPubkeyGlobal(pubkey) {
				// this user is an admin
				return true, nil
			} else {
				return false, nil
			}
		}
	}
	log.Println("invalid key format for key", privkey)
	return false, err
}

func (self httpModUI) MessageChan() chan NNTPMessage {
	return self.modMessageChan
}

func (self httpModUI) getSession(r *http.Request) *sessions.Session {
	s, _ := self.store.Get(r, "nntpchan-mod")
	return s
}

// get the session's private key as bytes or nil if we don't have it
func (self httpModUI) getSessionPrivkeyBytes(r *http.Request) []byte {
	s := self.getSession(r)
	k, ok := s.Values["privkey"]
	if ok {
		privkey_bytes, err := hex.DecodeString(k.(string))
		if err == nil {
			return privkey_bytes
		}
		log.Println("failed to decode private key bytes from session", err)
	} else {
		log.Println("failed to get private key from session, no private key in session?")
	}
	return nil
}

// returns true if the session is okay
// otherwise redirect to login page
func (self httpModUI) checkSession(r *http.Request) bool {
	s := self.getSession(r)
	k, ok := s.Values["privkey"]
	if ok {
		ok, err := self.CheckKey(k.(string))
		if err != nil {
			return false
		}
		return ok
	}
	return false
}

func (self httpModUI) writeTemplate(wr http.ResponseWriter, name string) {
	self.writeTemplateParam(wr, name, nil)
}

func (self httpModUI) writeTemplateParam(wr http.ResponseWriter, name string, param map[string]string) {
	if param == nil {
		param = make(map[string]string)
	}
	param["prefix"] = self.prefix
	param["mod_prefix"] = self.mod_prefix
	io.WriteString(wr, template.renderTemplate(name, param))
}

// do a function as authenticated
// pass in the request path to the handler
func (self httpModUI) asAuthed(handler func(string), wr http.ResponseWriter, r *http.Request) {
	if self.checkSession(r) {
		handler(r.URL.Path)
	} else {
		wr.WriteHeader(403)
	}
}

// do stuff to a certain message if with have it and are authed
func (self httpModUI) asAuthedWithMessage(handler func(ArticleEntry, *http.Request) map[string]interface{}, wr http.ResponseWriter, req *http.Request) {
	self.asAuthed(func(path string) {
		// get the long hash
		if strings.Count(path, "/") > 2 {
			// TOOD: prefix detection
			longhash := strings.Split(path, "/")[3]
			// get the message id
			msg, err := self.database.GetMessageIDByHash(longhash)
			resp := make(map[string]interface{})
			if err == nil {
				resp = handler(msg, req)
			} else {
				resp["error"] = fmt.Sprintf("don't have message with hash %s, %s", longhash, err.Error())
			}
			enc := json.NewEncoder(wr)
			enc.Encode(resp)
		} else {
			wr.WriteHeader(404)
		}
	}, wr, req)
}

func (self httpModUI) HandleAddPubkey(wr http.ResponseWriter, r *http.Request) {
}

func (self httpModUI) HandleDelPubkey(wr http.ResponseWriter, r *http.Request) {
}

func (self httpModUI) HandleUnbanAddress(wr http.ResponseWriter, r *http.Request) {
	self.asAuthed(func(path string) {
		// extract the ip address
		// TODO: ip ranges and prefix detection
		if strings.Count(path, "/") > 2 {
			addr := strings.Split(path, "/")[3]
			resp := make(map[string]interface{})
			banned, err := self.database.CheckIPBanned(addr)
			if err != nil {
				resp["error"] = fmt.Sprintf("cannot tell if %s is banned: %s", addr, err.Error())
			} else if banned {
				// TODO: rangebans
				err = self.database.UnbanAddr(addr)
				if err == nil {
					resp["result"] = fmt.Sprintf("%s was unbanned", addr)
				} else {
					resp["error"] = err.Error()
				}
			} else {
				resp["error"] = fmt.Sprintf("%s was not banned", addr)
			}
			enc := json.NewEncoder(wr)
			enc.Encode(resp)
		} else {
			wr.WriteHeader(404)
		}
	}, wr, r)
}

// handle ban logic
func (self httpModUI) handleBanAddress(msg ArticleEntry, r *http.Request) map[string]interface{} {
	// get the article headers
	resp := make(map[string]interface{})
	msgid := msg.MessageID()
	hdr := self.articles.GetHeaders(msgid)
	if hdr == nil {
		// we don't got it?!
		resp["error"] = fmt.Sprintf("message %s not on the filesystem wtf?", msgid)
	} else {
		// get the associated encrypted ip
		encip := hdr.Get("X-Encrypted-Ip", hdr.Get("X-Encrypted-IP", ""))
		encip = strings.Trim(encip, "\t ")

		if len(encip) == 0 {
			// no ip header detected
			resp["error"] = fmt.Sprintf("%s has no IP, ban Tor instead", msgid)
		} else {
			// get the ip address if we have it
			ip, err := self.database.GetIPAddress(encip)
			if len(ip) > 0 {
				// we have it
				// ban the address
				err = self.database.BanAddr(ip)
				// then we tell everyone about it
				var key string
				// TODO: we SHOULD have the key, but what if we do not?
				key, err = self.database.GetEncKey(encip)
				// create mod message
				// TODO: hardcoded ban period
				mm := ModMessage{overchanInetBan(encip, key, -1)}
				privkey_bytes := self.getSessionPrivkeyBytes(r)
				if privkey_bytes == nil {
					// this should not happen
					log.Println("failed to get privkey bytes from session")
					resp["error"] = "failed to get private key from session. wtf?"
				} else {
					// wrap and sign
					nntp := wrapModMessage(mm)
					nntp, err = signArticle(nntp, privkey_bytes)
					if err == nil {
						// federate
						self.modMessageChan <- nntp
					}
				}
			} else {
				// we don't have it
				// ban the encrypted version
				err = self.database.BanEncAddr(encip)
			}
			if err == nil {
				result_msg := fmt.Sprintf("We banned %s", encip)
				if len(ip) > 0 {
					result_msg += fmt.Sprintf(" (%s)", ip)
				}
				resp["banned"] = result_msg
			} else {
				resp["error"] = err.Error()
			}

		}
	}
	return resp
}

func (self httpModUI) handleDeletePost(msg ArticleEntry, r *http.Request) map[string]interface{} {
	var mm ModMessage
	resp := make(map[string]interface{})
	msgid := msg.MessageID()
	delmsgs := []string{}
	// get headers
	hdr := self.articles.GetHeaders(msgid)
	if hdr == nil {
		resp["error"] = fmt.Sprintf("message %s is not on the filesystem? wtf!", msgid)
	} else {
		ref := hdr.Get("References", hdr.Get("Reference", ""))
		ref = strings.Trim(ref, "\t ")
		// is it a root post?
		if ref == "" {
			// load replies
			replies := self.database.GetThreadReplies(msgid, 0)
			if replies != nil {
				for _, repl := range replies {
					// append mod line to mod message for reply
					mm = append(mm, overchanDelete(repl))
					// add to delete queue
					delmsgs = append(delmsgs, repl)
				}
			}
		}
		delmsgs = append(delmsgs, msgid)
		// append mod line to mod message
		mm = append(mm, overchanDelete(msgid))

		resp["deleted"] = delmsgs
		// only regen threads when we delete a non root port
		if ref != "" {
			group := hdr.Get("Newsgroups", "")
			self.regen(ArticleEntry{
				ref, group,
			})
		}
		privkey_bytes := self.getSessionPrivkeyBytes(r)
		if privkey_bytes == nil {
			// crap this should never happen
			log.Println("failed to get private keys from session, not federating")
		} else {
			// wrap and sign mod message
			nntp, err := signArticle(wrapModMessage(mm), privkey_bytes)
			if err == nil {
				// send it off to federate
				self.modMessageChan <- nntp
			} else {
				resp["error"] = fmt.Sprintf("signing error: %s", err.Error())
			}
		}
	}
	return resp
}

// ban the address of a poster
func (self httpModUI) HandleBanAddress(wr http.ResponseWriter, r *http.Request) {
	self.asAuthedWithMessage(self.handleBanAddress, wr, r)
}

// delete a post
func (self httpModUI) HandleDeletePost(wr http.ResponseWriter, r *http.Request) {
	self.asAuthedWithMessage(self.handleDeletePost, wr, r)
}

func (self httpModUI) HandleLogin(wr http.ResponseWriter, r *http.Request) {
	privkey := r.FormValue("privkey")
	msg := "failed login: "
	if len(privkey) == 0 {
		msg += "no key"
	} else {
		ok, err := self.CheckKey(privkey)
		if err != nil {
			msg += fmt.Sprintf("%s", err)
		} else if ok {
			msg = "login okay"
			sess := self.getSession(r)
			sess.Values["privkey"] = privkey
			sess.Save(r, wr)
		} else {
			msg += "invalid key"
		}
	}
	self.writeTemplateParam(wr, "modlogin_result.mustache", map[string]string{"message": msg})
}

func (self httpModUI) HandleKeyGen(wr http.ResponseWriter, r *http.Request) {
	pk, sk := newSignKeypair()
	tripcode := makeTripcode(pk)
	self.writeTemplateParam(wr, "keygen.mustache", map[string]string{"public": pk, "secret": sk, "tripcode": tripcode})
}

func (self httpModUI) ServeModPage(wr http.ResponseWriter, r *http.Request) {
	if self.checkSession(r) {
		// we are logged in
		// serve mod page
		self.writeTemplate(wr, "modpage.mustache")
	} else {
		// we are not logged in
		// serve login page
		self.writeTemplate(wr, "modlogin.mustache")
	}

}
