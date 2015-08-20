//
// markup.go 
// memeposting markup parser
//
package srnd

import (
  "html"
  "regexp"
  "strings"
)
// regex for code tags
var re_code_tag = regexp.MustCompile(`\[code\]([\s\S]*)\[/code\]`)

func memeposting(src string) string {
  // escape
  src = html.EscapeString(src)
  // find and format code tags
  src = re_code_tag.ReplaceAllString(src, "\n<pre>\n${1}\n</pre>\n")
  // make newlines
  markup := ""
  for _, line := range strings.Split(src, "\n") {
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
  
  // return
  return markup
}
