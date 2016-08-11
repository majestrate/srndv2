package webhooks

import (
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/nntp"
	"github.com/majestrate/srndv2/lib/nntp/message"
	"github.com/majestrate/srndv2/lib/store"

	"io"
	"io/ioutil"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

// web hook implementation
type httpWebhook struct {
	conf    *config.WebhookConfig
	storage store.Storage
	hdr     *message.HeaderIO
}

func (h *httpWebhook) SentArticleVia(msgid nntp.MessageID, name string) {
	// web hooks don't care about feed state
}

// we got a new article
func (h *httpWebhook) GotArticle(msgid nntp.MessageID, group nntp.Newsgroup) {
	f, err := h.storage.OpenArticle(msgid.String())
	if err == nil {
		c := textproto.NewConn(f)
		var hdr textproto.MIMEHeader
		hdr, err = c.ReadMIMEHeader()
		if err == nil {
			u, _ := url.Parse(h.conf.URL)
			q := u.Query()
			for k, vs := range hdr {
				for _, v := range vs {
					q.Add(k, v)
				}
			}
			u.RawQuery = q.Encode()
			ctype := hdr.Get("Content-Type")
			if ctype == "" {
				ctype = "text/plain"
			}
			ctype = strings.Replace(ctype, "multipart/mixed", "multipart/form-data", 1) 
			var r *http.Response
			r, err = http.Post(u.String(), ctype, c.R)
			if err == nil {
				_, err = io.Copy(ioutil.Discard, r.Body)
				r.Body.Close()
				log.Infof("hook called for %s", msgid)
			}
		} else {
			f.Close()
		}
	}
	if err != nil {
		log.Errorf("error calling web hook %s: %s", h.conf.Name, err.Error())
	}
}
