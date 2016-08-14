package webhooks

import (
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/nntp"
	"github.com/majestrate/srndv2/lib/nntp/message"
	"github.com/majestrate/srndv2/lib/store"

	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
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
			ctype := hdr.Get("Content-Type")
			if ctype == "" {
				ctype = "text/plain"
			}
			ctype = strings.Replace(strings.ToLower(ctype), "multipart/mixed", "multipart/form-data", 1)
			u, _ := url.Parse(h.conf.URL)
			q := u.Query()
			for k, vs := range hdr {
				for _, v := range vs {
					q.Add(k, v)
				}
			}
			q.Set("Content-Type", ctype)
			u.RawQuery = q.Encode()

			var body io.Reader

			if strings.HasPrefix(ctype, "multipart") {
				pr, pw := io.Pipe()
				log.Debug("using pipe")
				body = pr
				go func(in io.Reader, out io.WriteCloser) {
					_, params, _ := mime.ParseMediaType(ctype)
					if params == nil {
						// send as whatever lol
						io.Copy(out, in)
					} else {
						boundary, _ := params["boundary"]
						mpr := multipart.NewReader(in, boundary)
						mpw := multipart.NewWriter(out)
						mpw.SetBoundary(boundary)
						for {
							part, err := mpr.NextPart()
							if err == io.EOF {
								err = nil
								break
							} else if err == nil {
								// get part header
								h := part.Header
								// rewrite header part for php
								cd := h.Get("Content-Disposition")
								r := regexp.MustCompile(`; name=".*"`)
								cd = r.ReplaceAllString(cd, `; name="attachment[]";`)
								log.Debug(cd)
								h.Set("Content-Disposition", cd)
								// make write part
								wp, err := mpw.CreatePart(h)
								if err == nil {
									// write part out
									io.Copy(wp, part)
								} else {
									log.Errorf("error writng webhook part: %s", err.Error())
								}
							}
							part.Close()
						}
						mpw.Close()
					}
					out.Close()
				}(c.R, pw)
			} else {
				body = c.R
			}

			var r *http.Response
			r, err = http.Post(u.String(), ctype, body)
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
