//
// tests.go -- unit tests
//

package srnd


import (
  "testing"
)


func TestSignVerify(t *testing.T) {
  // create article store
  store := articleStore{
    directory: "test_articles",
    temp: "test_articles_tmp",
    attachments: "test_attachments",
    thumbs: "test_thumbnails",
  }
  store.Init()
  
  t.Logf("create message")
  
}
