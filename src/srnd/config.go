//
// config.go
//

package srnd

import (
	"encoding/base32"
	"fmt"
	"github.com/majestrate/configparser"
	"github.com/majestrate/nacl"
	"log"
	"net"
	"path/filepath"
	"strings"
	"time"
)

type FeedConfig struct {
	policy           FeedPolicy
	quarks           map[string]string
	Addr             string
	sync             bool
	proxy_type       string
	proxy_addr       string
	username         string
	passwd           string
	linkauth_keyfile string
	tls_off          bool
	Name             string
	sync_interval    time.Duration
}

type APIConfig struct {
	srndAddr     string
	frontendAddr string
}

type CryptoConfig struct {
	privkey_file string
	cert_file    string
	hostname     string
	cert_dir     string
}

// pprof settings
type ProfilingConfig struct {
	bind   string
	enable bool
}

type SRNdConfig struct {
	daemon   map[string]string
	crypto   *CryptoConfig
	store    map[string]string
	database map[string]string
	cache    map[string]string
	feeds    []FeedConfig
	frontend map[string]string
	system   map[string]string
	worker   map[string]string
	pprof    *ProfilingConfig
}

// check for config files
// generate defaults on demand
func CheckConfig() {
	if !CheckFile("srnd.ini") {
		var conf *configparser.Configuration
		if !InstallerEnabled() {
			log.Println("no srnd.ini, creating...")
			conf = GenSRNdConfig()
		} else {
			res := make(chan *configparser.Configuration)
			installer := NewInstaller(res)
			go installer.Start()
			conf = <-res
			installer.Stop()
			close(res)
		}
		err := configparser.Save(conf, "srnd.ini")
		if err != nil {
			log.Fatal("cannot generate srnd.ini", err)
		}
	}
	if !CheckFile("feeds.ini") {
		log.Println("no feeds.ini, creating...")
		err := GenFeedsConfig()
		if err != nil {
			log.Fatal("cannot generate feeds.ini", err)
		}
	}
}

// generate default feeds.ini
func GenFeedsConfig() error {
	conf := configparser.NewConfiguration()
	sect := conf.NewSection("feed-dummy")
	sect.Add("proxy-type", "socks4a")
	sect.Add("proxy-host", "127.0.0.1")
	sect.Add("proxy-port", "9050")
	sect.Add("host", "dummy")
	sect.Add("port", "119")

	sect = conf.NewSection("dummy")
	sect.Add("overchan.*", "1")
	sect.Add("ano.paste", "0")
	sect.Add("ctl", "1")

	return configparser.Save(conf, "feeds.ini")
}

// generate default srnd.ini
func GenSRNdConfig() *configparser.Configuration {
	conf := configparser.NewConfiguration()

	// nntp related section
	sect := conf.NewSection("nntp")
	sect.Add("instance_name", "test.srndv2.tld")
	sect.Add("bind", "127.0.0.1:1199")
	sect.Add("sync_on_start", "1")
	sect.Add("allow_anon", "0")
	sect.Add("allow_anon_attachments", "0")
	sect.Add("allow_attachments", "1")
	sect.Add("require_tls", "1")
	sect.Add("anon_nntp", "0")

	// profiling settings
	sect = conf.NewSection("pprof")
	sect.Add("enable", "0")
	sect.Add("bind", "127.0.0.1:17000")

	// crypto related section
	sect = conf.NewSection("crypto")
	sect.Add("tls-keyname", "overchan")
	sect.Add("tls-hostname", "!!put-hostname-or-ip-of-server-here")
	sect.Add("tls-trust-dir", "certs")

	// article store section
	sect = conf.NewSection("articles")

	sect.Add("store_dir", "articles")
	sect.Add("incoming_dir", "/tmp/articles")
	sect.Add("attachments_dir", "webroot/img")
	sect.Add("thumbs_dir", "webroot/thm")
	sect.Add("convert_bin", "/usr/bin/convert")
	sect.Add("ffmpegthumbnailer_bin", "/usr/bin/ffmpeg")
	sect.Add("sox_bin", "/usr/bin/sox")
	sect.Add("compression", "0")

	// database backend config
	sect = conf.NewSection("database")
	// defaults to redis if enabled
	if RedisEnabled() {
		sect.Add("type", "redis")
		sect.Add("schema", "single")
		sect.Add("host", "localhost")
		sect.Add("port", "6379")
		sect.Add("user", "")
		sect.Add("password", "")
	} else {
		// otherwise defaults to postgres
		sect.Add("type", "postgres")
		sect.Add("schema", "srnd")
		sect.Add("host", "/var/run/postgresql")
		sect.Add("port", "")
		sect.Add("user", "")
		sect.Add("password", "")
	}

	// cache backend config
	sect = conf.NewSection("cache")
	// defaults to file
	sect.Add("type", "file")

	// baked in static html frontend
	sect = conf.NewSection("frontend")
	sect.Add("enable", "1")
	sect.Add("allow_files", "1")
	sect.Add("regen_on_start", "0")
	sect.Add("regen_threads", "1")
	sect.Add("bind", "[::]:18000")
	sect.Add("name", "web.srndv2.test")
	sect.Add("webroot", "webroot")
	sect.Add("minimize_html", "0")
	sect.Add("prefix", "/")
	sect.Add("static_files", "contrib")
	sect.Add("templates", "contrib/templates/default")
	sect.Add("translations", "contrib/translations")
	sect.Add("locale", "en")
	sect.Add("domain", "localhost")
	sect.Add("json-api", "0")
	sect.Add("json-api-username", "fucking-change-this-value")
	sect.Add("json-api-password", "seriously-fucking-change-this-value")
	secret_bytes := nacl.RandBytes(8)
	secret := base32.StdEncoding.EncodeToString(secret_bytes)
	sect.Add("api-secret", secret)

	return conf
}

// save a list of feeds to overwrite feeds.ini
func SaveFeeds(feeds []FeedConfig) (err error) {
	conf := configparser.NewConfiguration()
	for _, feed := range feeds {
		if len(feed.Name) == 0 {
			// don't do feed with no name
			continue
		}
		sect := conf.NewSection("feed-" + feed.Name)
		if len(feed.proxy_type) > 0 {
			sect.Add("proxy-type", feed.proxy_type)
		}
		phost, pport, _ := net.SplitHostPort(feed.proxy_addr)
		sect.Add("proxy-host", phost)
		sect.Add("proxy-port", pport)
		host, port, _ := net.SplitHostPort(feed.Addr)
		sect.Add("host", host)
		sect.Add("port", port)
		sync := "0"
		if feed.sync {
			sync = "1"
		}
		sect.Add("sync", sync)
		interval := feed.sync_interval / time.Second
		sect.Add("sync-interval", fmt.Sprintf("%d", int(interval)))
		sect.Add("username", feed.username)
		sect.Add("password", feed.passwd)
		sect = conf.NewSection(feed.Name)
		for k, v := range feed.policy.rules {
			sect.Add(k, v)
		}
	}
	return configparser.Save(conf, "feeds.ini")
}

// read config files
func ReadConfig() *SRNdConfig {

	// begin read srnd.ini

	fname := "srnd.ini"
	var s *configparser.Section
	conf, err := configparser.Read(fname)
	if err != nil {
		log.Fatal("cannot read config file", fname)
		return nil
	}
	var sconf SRNdConfig

	s, err = conf.Section("pprof")
	if err == nil {
		opts := s.Options()
		sconf.pprof = new(ProfilingConfig)
		sconf.pprof.enable = opts["enable"] == "1"
		sconf.pprof.bind = opts["bind"]
	}

	s, err = conf.Section("crypto")
	if err == nil {
		opts := s.Options()
		sconf.crypto = new(CryptoConfig)
		k := opts["tls-keyname"]
		h := opts["tls-hostname"]
		if strings.HasPrefix(h, "!") || len(h) == 0 {
			log.Fatal("please set tls-hostname to be the hostname or ip address of your server")
		} else {
			sconf.crypto.hostname = h
			sconf.crypto.privkey_file = k + "-" + h + ".key"
			sconf.crypto.cert_dir = opts["tls-trust-dir"]
			sconf.crypto.cert_file = filepath.Join(sconf.crypto.cert_dir, k+"-"+h+".crt")
		}
	} else {
		// we have no crypto section
		log.Println("!!! we will not use encryption for nntp as no crypto section is specified in srnd.ini")
	}
	s, err = conf.Section("nntp")
	if err != nil {
		log.Println("no section 'nntp' in srnd.ini")
		return nil
	}

	sconf.daemon = s.Options()

	s, err = conf.Section("database")
	if err != nil {
		log.Println("no section 'database' in srnd.ini")
		return nil
	}

	sconf.database = s.Options()

	s, err = conf.Section("cache")
	if err != nil {
		log.Println("no section 'cache' in srnd.ini")
		log.Println("falling back to default cache config")
		sconf.cache = make(map[string]string)
		sconf.cache["type"] = "file"
	} else {
		sconf.cache = s.Options()
	}

	s, err = conf.Section("articles")
	if err != nil {
		log.Println("no section 'articles' in srnd.ini")
		return nil
	}

	sconf.store = s.Options()

	// frontend config

	s, err = conf.Section("frontend")

	if err != nil {
		log.Println("no frontend section in srnd.ini, disabling frontend")
		sconf.frontend = make(map[string]string)
		sconf.frontend["enable"] = "0"
	} else {
		log.Println("frontend configured in srnd.ini")
		sconf.frontend = s.Options()
		_, ok := sconf.frontend["enable"]
		if !ok {
			// default to "0"
			sconf.frontend["enable"] = "0"
		}
		enable, _ := sconf.frontend["enable"]
		if enable == "1" {
			log.Println("frontend enabled in srnd.ini")
		} else {
			log.Println("frontend not enabled in srnd.ini, disabling frontend")
		}
	}

	// begin load feeds.ini

	fname = "feeds.ini"
	conf, err = configparser.Read(fname)

	if err != nil {
		log.Fatal("cannot read config file", fname)
		return nil
	}

	sections, err := conf.Find("feed-*")
	if err != nil {
		log.Fatal("failed to load feeds.ini", err)
	}

	var num_sections int
	num_sections = len(sections)

	if num_sections > 0 {
		sconf.feeds = make([]FeedConfig, num_sections)
		idx := 0

		// load feeds
		for _, sect := range sections {
			var fconf FeedConfig
			// check for proxy settings
			val := sect.ValueOf("proxy-type")
			if len(val) > 0 && strings.ToLower(val) != "none" {
				fconf.proxy_type = strings.ToLower(val)
				proxy_host := sect.ValueOf("proxy-host")
				proxy_port := sect.ValueOf("proxy-port")
				fconf.proxy_addr = strings.Trim(proxy_host, " ") + ":" + strings.Trim(proxy_port, " ")
			}

			host := sect.ValueOf("host")
			port := sect.ValueOf("port")

			// check to see if we want to sync with them first
			val = sect.ValueOf("sync")
			if val == "1" {
				fconf.sync = true
				// sync interval in seconds
				i := mapGetInt(sect.Options(), "sync-interval", 60)
				if i < 60 {
					i = 60
				}
				fconf.sync_interval = time.Second * time.Duration(i)
			}

			// username / password auth
			fconf.username = sect.ValueOf("username")
			fconf.passwd = sect.ValueOf("password")
			fconf.tls_off = sect.ValueOf("disabletls") == "1"

			// load feed polcies
			sect_name := sect.Name()[5:]
			fconf.Name = sect_name
			if len(host) > 0 && len(port) > 0 {
				// host port specified
				fconf.Addr = host + ":" + port
			} else {
				// no host / port specified
				fconf.Addr = strings.Trim(sect_name, " ")
			}
			feed_sect, err := conf.Section(sect_name)
			if err != nil {
				log.Fatal("no section", sect_name, "in feeds.ini")
			}
			opts := feed_sect.Options()
			fconf.policy.rules = make(map[string]string)
			for k, v := range opts {
				fconf.policy.rules[k] = v
			}
			sconf.feeds[idx] = fconf
			idx += 1
		}
	}

	// feed quarks
	sections, err = conf.Find("quarks-*")
	if err == nil {
		// we have quarks? neat, let's load them
		// does not check for anything specific
		for _, sect := range sections {
			sect_name := sect.Name()[7:]
			// find the feed for this quark
			for idx, fconf := range sconf.feeds {
				if fconf.Addr == sect_name {
					// yup this is the one
					sconf.feeds[idx].quarks = sect.Options()
				}
			}
		}
	}

	return &sconf
}

// fatals on failed validation
func (self *SRNdConfig) Validate() {
	// check for daemon section entries
	daemon_param := []string{"bind", "instance_name", "allow_anon", "allow_anon_attachments"}
	for _, p := range daemon_param {
		_, ok := self.daemon[p]
		if !ok {
			log.Fatalf("in section [nntp], no parameter '%s' provided", p)
		}
	}

	// check validity of store directories
	store_dirs := []string{"store", "incoming", "attachments", "thumbs"}
	for _, d := range store_dirs {
		k := d + "_dir"
		_, ok := self.store[k]
		if !ok {
			log.Fatalf("in section [store], no parameter '%s' provided", k)
		}
	}

	// check database parameters existing
	db_param := []string{"host", "port", "user", "password", "type", "schema"}
	for _, p := range db_param {
		_, ok := self.database[p]
		if !ok {
			log.Fatalf("in section [database], no parameter '%s' provided", p)
		}
	}
}
