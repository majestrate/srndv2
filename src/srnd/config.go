//
// config.go
//

package srnd

import (
  "encoding/base32"
  "github.com/majestrate/configparser"
  "github.com/majestrate/srndv2/src/nacl"
  "log"
  "strings"
)

type FeedConfig struct {
  policy FeedPolicy
  quarks map[string]string
  addr string
  sync bool
  proxy_type string
  proxy_addr string
  linkauth_keyfile string
}

type APIConfig struct {
  srndAddr string
  frontendAddr string
}
type SRNdConfig struct { 
  daemon map[string]string
  store map[string]string
  database map[string]string
  feeds []FeedConfig
  frontend map[string]string
  system map[string]string
  worker map[string]string
}

// check for config files
// generate defaults on demand
func CheckConfig() {
  if ! CheckFile("srnd.ini") {
    log.Println("no srnd.ini, creating...")
    err := GenSRNdConfig()
    if err != nil {
      log.Fatal("cannot generate srnd.ini", err)
    }
  }
  if ! CheckFile("feeds.ini") {
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
  sect := conf.NewSection("feed-10.0.0.1:119")
  sect.Add("proxy-type", "socks4a")
  sect.Add("proxy-host", "127.0.0.1")
  sect.Add("proxy-port", "9050")

  sect = conf.NewSection("10.0.0.1:119")
  sect.Add("overchan.*", "1")
  sect.Add("ano.paste", "0")
  sect.Add("ctl", "1")
 
  return configparser.Save(conf, "feeds.ini")
}

// generate default srnd.ini
func GenSRNdConfig() error {
  conf := configparser.NewConfiguration()
  
  // nntp related section
  sect := conf.NewSection("nntp")
  sect.Add("instance_name", "test.srndv2.tld")
  sect.Add("bind", "127.0.0.1:1199")
  sect.Add("sync_on_start", "1")
  sect.Add("allow_anon", "0")
  sect.Add("allow_anon_attachments", "0")

  // article store section
  sect = conf.NewSection("articles")

  sect.Add("store_dir", "articles")
  sect.Add("incoming_dir", "/tmp/articles")
  sect.Add("attachments_dir", "webroot/img")
  sect.Add("thumbs_dir", "webroot/thm")
  sect.Add("convert_bin", "/usr/bin/convert")
  sect.Add("ffmpegthumbnailer_bin", "/usr/bin/ffmpegthumbnailer")
  sect.Add("sox_bin", "/usr/bin/sox")
  
  // database backend config
  sect = conf.NewSection("database")

  // change this to mysql to use with mariadb or mysql
  sect.Add("type", "postgres")
  // change this to infinity to use with infinity-next
  sect.Add("schema", "srnd")
  sect.Add("host", "/var/run/postgresql")
  sect.Add("port", "5432")
  sect.Add("user", "srnd")
  sect.Add("password", "srnd")
  
  // baked in static html frontend
  sect = conf.NewSection("frontend")
  sect.Add("enable", "1")
  sect.Add("allow_files", "1")
  sect.Add("regen_on_start", "0")
  sect.Add("regen_threads", "1")
  sect.Add("nntp", "[::]:1119")
  sect.Add("bind", "[::]:18000")
  sect.Add("name", "web.srndv2.test")
  sect.Add("webroot", "webroot")
  sect.Add("prefix", "/")
  sect.Add("static_files", "contrib")
  sect.Add("templates", "contrib/templates/default")
  sect.Add("domain", "localhost")
  secret_bytes := nacl.RandBytes(8)
  secret := base32.StdEncoding.EncodeToString(secret_bytes)
  sect.Add("api-secret", secret)
  
  return configparser.Save(conf, "srnd.ini")
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
  var sconf SRNdConfig;

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
    _ , ok := sconf.frontend["enable"]
    if ! ok {
      // default to "0"
      sconf.frontend["enable"] = "0"
    }
    enable , _ := sconf.frontend["enable"]
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

      // check to see if we want to sync with them first
      val = sect.ValueOf("sync")
      if val == "1" {
        fconf.sync = true
      }
      
      // load feed polcies
      sect_name :=  sect.Name()[5:]
      fconf.addr = strings.Trim(sect_name, " ")
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
        if fconf.addr == sect_name {
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
    if ! ok {
      log.Fatalf("in section [nntp], no parameter '%s' provided", p)
    }
  }
  
  // check validity of store directories
  store_dirs := []string{"store", "incoming", "attachments", "thumbs"}
  for _, d := range store_dirs {
    k := d + "_dir"
    _, ok := self.store[k]
    if ! ok {
      log.Fatalf("in section [store], no parameter '%s' provided", k)
    }
  }

  // check database parameters existing
  db_param := []string{"host", "port", "user", "password", "type", "schema"}
  for _, p := range db_param {
    _, ok := self.database[p]
    if ! ok {
      log.Fatalf("in section [database], no parameter '%s' provided", p)
    }
  }  
}
