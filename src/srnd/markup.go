//
// markup.go 
// memeposting markup parser
//
package srnd

import (
  "html"
  "strings"
)

func memeposting(src string) string {
  // escape
  src = html.EscapeString(src)
  // make newlines
  found_code_tag := false
  code_content := ""
  markup := ""
  for _, line := range strings.Split(src, "\n") {
    if strings.Count(line, "[code]") == 1 {
      found_code_tag = true
      code_content = strings.Split(line, "[code]")[0]
    }
    if found_code_tag {
      code_content += line
      if strings.Count(line, "[/code]") == 1 {
        found_code_tag = false
        markup += "<pre>"
        markup += strings.Trim(code_content, "[/code]")
        markup += "</pre>"
        code_content = ""
      } else {
        continue
      }
    } else {
      // check for meme arrows
      if strings.HasPrefix(line, "&gt;") {
        markup += "<p><span class='memearrows'>"
        markup += line
        markup += "</span></p>"
      } else {
        markup += "<p>"
        markup += line
        markup += "</p>"
      }
    }
  }
  
  // return
  return markup
}
