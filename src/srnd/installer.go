package srnd

import (
	"database/sql"
	"fmt"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/majestrate/configparser"
	"golang.org/x/text/language"
	"gopkg.in/redis.v3"
	"gopkg.in/tylerb/graceful.v1"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"time"
)

type handlePost func(*dialogNode, url.Values, *configparser.Configuration) (*dialogNode, error)
type templateModel map[string]interface{}
type prepareModel func(*dialogNode, error, *configparser.Configuration) templateModel

type dialogNode struct {
	parent   *dialogNode
	children map[string]*dialogNode

	post  handlePost
	model prepareModel

	templateName string
}

type Installer struct {
	root            *dialogNode
	currentNode     *dialogNode
	currentErr      error
	result          chan *configparser.Configuration
	config          *configparser.Configuration
	srv             *graceful.Server
	hasTranslations bool
}

func handleDBTypePost(self *dialogNode, form url.Values, conf *configparser.Configuration) (*dialogNode, error) {
	db := form.Get("db")
	log.Println("DB chosen: ", db)
	if db == "redis" {
		return self.children["redis"], nil
	}
	if db == "postgres" {
		return self.children["postgres"], nil
	}
	return self, nil
}

func prepareDefaultModel(self *dialogNode, err error, conf *configparser.Configuration) templateModel {
	param := make(map[string]interface{})
	param["dialog"] = &BaseDialogModel{ErrorModel{err}, StepModel{self}}
	return param
}

func handleRedisDBPost(self *dialogNode, form url.Values, conf *configparser.Configuration) (*dialogNode, error) {
	if form.Get("back") == "true" {
		return self.parent, nil
	}
	sect, _ := conf.Section("database")
	host := form.Get("host")
	port := form.Get("port")
	passwd := form.Get("password")

	err := checkRedisConnection(host, port, passwd)
	if err != nil {
		return self, err
	}
	sect.Add("type", "redis")
	sect.Add("schema", "single")
	sect.Add("host", host)
	sect.Add("port", port)
	sect.Add("password", passwd)

	return self.children["next"], nil
}

func prepareRedisDBModel(self *dialogNode, err error, conf *configparser.Configuration) templateModel {
	param := make(map[string]interface{})
	sect, _ := conf.Section("database")
	host := sect.ValueOf("host")
	port := sect.ValueOf("port")
	param["dialog"] = &DBModel{ErrorModel{err}, StepModel{self}, "", host, port}
	return param
}

func handlePostgresDBPost(self *dialogNode, form url.Values, conf *configparser.Configuration) (*dialogNode, error) {
	if form.Get("back") == "true" {
		return self.parent, nil
	}
	sect, _ := conf.Section("database")
	host := form.Get("host")
	port := form.Get("port")
	passwd := form.Get("password")
	user := form.Get("user")

	err := checkPostgresConnection(host, port, user, passwd)
	if err != nil {
		return self, err
	}
	sect.Add("type", "postgres")
	sect.Add("schema", "srnd")
	sect.Add("host", host)
	sect.Add("port", port)
	sect.Add("password", passwd)
	sect.Add("user", user)

	return self.children["next"], nil
}

func preparePostgresDBModel(self *dialogNode, err error, conf *configparser.Configuration) templateModel {
	param := make(map[string]interface{})
	sect, _ := conf.Section("database")
	host := sect.ValueOf("host")
	port := sect.ValueOf("port")
	user := sect.ValueOf("user")
	param["dialog"] = &DBModel{ErrorModel{err}, StepModel{self}, user, host, port}
	return param
}

func (self *Installer) HandleInstallerGet(wr http.ResponseWriter, r *http.Request) {
	if !self.hasTranslations {
		t, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
		locale := ""
		if len(t) > 0 {
			locale = t[0].String()
		}
		InitI18n(locale, filepath.Join("contrib", "translations"))
		self.hasTranslations = true
	}
	m := self.currentNode.model(self.currentNode, self.currentErr, self.config)
	template.writeTemplate(self.currentNode.templateName, m, wr)
}

func (self *Installer) HandleInstallerPost(wr http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err == nil {
		next, newErr := self.currentNode.post(self.currentNode, r.PostForm, self.config)
		if next == nil {
			self.result <- self.config
			//defer self.srv.Stop(10 * time.Second)
		}
		self.currentNode = next
		self.currentErr = newErr

		http.Redirect(wr, r, r.URL.String(), http.StatusSeeOther) //redirect to the same url, but with a GET
		return
	}
	http.Error(wr, "Bad Request", http.StatusBadRequest)
}

func NewInstaller(result chan *configparser.Configuration) *Installer {
	inst := new(Installer)
	inst.root = initInstallerTree()
	inst.currentNode = inst.root
	inst.result = result
	inst.config = GenSRNdConfig()
	inst.hasTranslations = false

	m := mux.NewRouter()
	m.Path("/").HandlerFunc(inst.HandleInstallerGet).Methods("GET")
	m.Path("/").HandlerFunc(inst.HandleInstallerPost).Methods("POST")

	inst.srv = &graceful.Server{
		Timeout:          10 * time.Second,
		NoSignalHandling: true,

		Server: &http.Server{
			Addr:    ":18000",
			Handler: m,
		},
	}

	return inst
}

func initInstallerTree() *dialogNode {
	root := &dialogNode{
		parent:       nil,
		children:     make(map[string]*dialogNode),
		post:         handleDBTypePost,
		model:        prepareDefaultModel,
		templateName: "inst_db.mustache",
	}

	redisDB := &dialogNode{
		parent:       root,
		children:     make(map[string]*dialogNode),
		post:         handleRedisDBPost,
		model:        prepareRedisDBModel,
		templateName: "inst_redis_db.mustache",
	}
	root.children["redis"] = redisDB

	postgresDB := &dialogNode{
		parent:       root,
		children:     make(map[string]*dialogNode),
		post:         handlePostgresDBPost,
		model:        preparePostgresDBModel,
		templateName: "inst_postgres_db.mustache",
	}
	root.children["postgres"] = postgresDB

	return root
}

func checkRedisConnection(host, port, passwd string) error {
	client := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(host, port),
		Password: passwd,
	})
	defer client.Close()

	_, err := client.Ping().Result() //check for successful connection
	return err
}

func checkPostgresConnection(host, port, user, password string) error {
	var db_str string
	if len(user) > 0 {
		if len(password) > 0 {
			db_str = fmt.Sprintf("user=%s password=%s host=%s port=%s client_encoding='UTF8' connect_timeout=3", user, password, host, port)
		} else {
			db_str = fmt.Sprintf("user=%s host=%s port=%s client_encoding='UTF8' connect_timeout=3", user, host, port)
		}
	} else {
		if len(port) > 0 {
			db_str = fmt.Sprintf("host=%s port=%s client_encoding='UTF8' connect_timeout=3", host, port)
		} else {
			db_str = fmt.Sprintf("host=%s client_encoding='UTF8' connect_timeout=3", host)
		}
	}

	conn, err := sql.Open("postgres", db_str)
	defer conn.Close()

	if err == nil {
		_, err = conn.Exec("SELECT datname FROM pg_database")
	}

	return err
}

func (self *Installer) Start() {
	self.srv.ListenAndServe()
}

func (self *Installer) Stop() {
	self.srv.Stop(1 * time.Second)
}

func InstallerEnabled() bool {
	return true
}
