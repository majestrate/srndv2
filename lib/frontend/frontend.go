package frontend

import (
	"github.com/majestrate/srndv2/lib/cache"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/database"
	"github.com/majestrate/srndv2/lib/model"
	"github.com/majestrate/srndv2/lib/nntp"

	"net"
	"strings"
)

// a frontend that displays nntp posts and allows posting
type Frontend interface {

	// run mainloop using net.Listener
	Serve(l net.Listener) error

	// do we accept this inbound post?
	AllowPost(p model.PostReference) bool

	// trigger a manual regen of indexes for a root post
	Regen(p model.PostReference)

	// implements nntp.EventHooks
	GotArticle(msgid nntp.MessageID, group nntp.Newsgroup)

	// implements nntp.EventHooks
	SentArticleVia(msgid nntp.MessageID, feedname string)
}

// create a new http frontend give frontend config
func NewHTTPFrontend(c *config.FrontendConfig, db database.DB) (f Frontend, err error) {

	// middlware cache
	var markupCache cache.CacheInterface
	// set up cache
	if c.Cache != nil {
		// get cache backend
		cacheBackend := strings.ToLower(c.Cache.Backend)
		if cacheBackend == "redis" {
			// redis cache
			markupCache, err = cache.NewRedisCache(c.Cache.Addr, c.Cache.Password)
			if err != nil {
				f = nil
				// error creating cache
				return
			}
			// redis cache backend was created
		} else {
			// fall through
		}
	}

	if markupCache == nil {
		// fallback cache backend is null cache
		markupCache = cache.NewNullCache()
	}

	var mid Middleware
	if c.Middleware != nil {
		// middleware configured
		mid, err = OverchanMiddleware(c.Middleware, markupCache, db)
		// error will fall through
	}

	if err == nil {
		// create http frontend only if no previous errors
		f, err = createHttpFrontend(c, mid, db)
	}
	return
}
