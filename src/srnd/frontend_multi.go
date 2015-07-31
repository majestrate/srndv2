//
// frontend_multi.go
// frontend multiplexer
//

package srnd

// muxed frontend for holding many frontends
type multiFrontend struct {
  muxedpostchan chan NNTPMessage
  muxedchan chan NNTPMessage
  frontends []Frontend
}


func (self multiFrontend) AllowNewsgroup(newsgroup string) bool {
  return true
}

func (self multiFrontend) Regen(msg ArticleEntry) {
  for _, front := range self.frontends {
    front.Regen(msg)
  }
}

func (self multiFrontend) Mainloop() {
  for idx := range(self.frontends) {
    go self.frontends[idx].Mainloop()
    go self.forwardPosts(self.frontends[idx])
  }
  

  // poll for incoming 
  chnl := self.PostsChan()
  for {
    select {
    case nntp := <- chnl:
      for _ , frontend := range self.frontends {
        ch := frontend.PostsChan()
        ch <- nntp
      }
      break
    }
  }
}

func (self multiFrontend) forwardPosts(front Frontend) {
  chnl := front.NewPostsChan()
  for {
    select {
    case nntp := <- chnl:
      // put in the path header the fact that this passed through the multifrontend
      // why? because why not.
      nntp = nntp.AppendPath("srndv2.frontend.mux")
      self.muxedpostchan <- nntp
    }
  }
}

func (self multiFrontend) NewPostsChan() chan NNTPMessage {
  return self.muxedpostchan
}

func (self multiFrontend) PostsChan() chan NNTPMessage {
  return self.muxedchan
}


func MuxFrontends(fronts ...Frontend) Frontend {
  var front multiFrontend
  front.muxedchan = make(chan NNTPMessage, 64)
  front.muxedpostchan = make(chan NNTPMessage, 64)
  front.frontends = fronts
  return front
}
