package srnd

import (
	"net/http"
	"net/url"
	"log"
	"time"
	"gopkg.in/tylerb/graceful.v1"
	"github.com/gorilla/mux"
	"github.com/majestrate/configparser"
	"golang.org/x/text/language"
	"path/filepath"
)

type handlePost func(*dialogNode, url.Values, *configparser.Configuration) (*dialogNode, error)
type templateModel map[string]interface{}
type prepareModel func(*dialogNode, error) templateModel

type dialogNode struct {
	parent *dialogNode
	children map[string]*dialogNode
	
	post handlePost
	model prepareModel
	
	templateName string
}

type Installer struct {
	root *dialogNode
	currentNode *dialogNode
	currentErr error
	result chan *configparser.Configuration
	config *configparser.Configuration
	srv *graceful.Server
	hasTranslations bool
}


func handleDBTypePost(self *dialogNode, form url.Values, conf *configparser.Configuration) (*dialogNode, error) {
	log.Println("DB chosen: ", form.Get("db"))
	return nil, nil
}

func prepareDefaultModel(self *dialogNode, err error) templateModel {
	param := make(map[string]interface{})
	param["dialog"] = BaseDialogModel{ErrorModel{err}, StepModel{self}}
	return param
}

func (self *Installer) HandleInstallerGet(wr http.ResponseWriter, r *http.Request) {
	if !self.hasTranslations {
		t, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
		locale:=""
		if len(t)>0 {
			locale=t[0].String()
		}
		InitI18n(locale, filepath.Join("contrib", "translations"))
		self.hasTranslations = true
	}
	m := self.currentNode.model(self.currentNode, self.currentErr)
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
		self.currentNode=next
		self.currentErr=newErr
		
		http.Redirect(wr, r, r.URL.String(), http.StatusSeeOther) //redirect to the same url, but with a GET
		return
	}
	http.Error(wr, "Bad Request", http.StatusBadRequest)
}

func NewInstaller(result chan *configparser.Configuration) *Installer {
	inst := new(Installer)
	inst.root = initInstallerTree()
	inst.currentNode= inst.root
	inst.result = result
	inst.config = GenSRNdConfig()
	inst.hasTranslations = false
	
	m := mux.NewRouter()
	m.Path("/").HandlerFunc(inst.HandleInstallerGet).Methods("GET")
	m.Path("/").HandlerFunc(inst.HandleInstallerPost).Methods("POST")
	
	inst.srv = &graceful.Server{
		Timeout: 10 * time.Second,
		NoSignalHandling: true,

		Server: &http.Server{
			Addr: ":18000",
			Handler: m,
		},
	}
	
	return inst
}

func initInstallerTree() *dialogNode {
	root := &dialogNode{
		parent: nil,
		post: handleDBTypePost,
		model: prepareDefaultModel,
		templateName: "inst_db.mustache",
	}
	return root
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

