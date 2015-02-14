//
// config.go
//

package srnd

import (
	"github.com/alyu/configparser"
)

// generate default config
func GenConfig(fname string) error {
	conf := configparser.NewConfiguration()
	
	sect := conf.NewSection("srnd")

	sect.Add("instance_name", "test.srndv2.tld")
	sect.Add("bind_host", "::1")
	sect.Add("bind_port", "1199")
	sect.Add("sync_on_start", "1")
	
	sect = conf.NewSection("store")

	sect.Add("base_dir", "articles")

	sect = conf.NewSection("database")

	sect.Add("type", "postgres")
	sect.Add("host", "127.0.0.1")
	sect.Add("port", "5432")
	sect.Add("user", "root")
	sect.Add("password", "root")

	return configparser.Save(conf, fname)
}
