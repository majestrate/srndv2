//
// policy.go
//
package main

import (
  "log"
  "regexp"
)

type FeedPolicy struct {
  rules map[string]string
}

func (self *FeedPolicy) FederateNewsgroup(newsgroup string) bool {
  var k string
  for k = range self.rules {
    if k[0] == '!' {
      continue
    }
    match , err := regexp.MatchString(k, newsgroup)
    if err != nil {
      log.Fatal(err) 
    }
    if match {
      return self.rules[k] == "1"
    }
  }
  // cynical, reject unknown
  return false
}

func (self *FeedPolicy) AllowsNewsgroup(newsgroup string) bool {
  var k string
  for k = range self.rules {
    
    inverse := k[0] == '!'
    if inverse {
      k = k[1:]
    }
    match, err := regexp.MatchString(k, newsgroup)
    if err != nil {
      log.Fatal(err)
    }
    if match {
      if inverse {
        return false
      }
      return true
    }
  }
  return false
}
