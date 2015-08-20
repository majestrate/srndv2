//
// markup.go 
// memeposting markup parser
//
package srnd

import (
  "html"
  "strings"
)
// meme arrow a line
func memeline(line string) (markup string) {
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
  return
}

func memeposting(src string) (markup string) {
  // escape
  src = html.EscapeString(src)

  found_code_tag := false
  code_content := ""
  // for each line...
  for _, line := range strings.Split(src, "\n") {
    // beginning of code tag ?
    if strings.Count(line, "[code]") > 0 {
      // yes there's a code tag
      found_code_tag = true
    }
    if found_code_tag {
      // collect content of code tag
      code_content += line + "\n"
      // end of code tag ?
      if strings.Count(line, "[/code]") == 1 {
        // yah
        found_code_tag = false
        // open pre tag
        markup += "<pre>"
        // remove open tag, only once so we can have a code tag verbatum inside
        code_content = strings.Replace(code_content, "[code]", "", 1)
        // remove all close tags, should only have 1
        code_content = strings.Replace(code_content, "[/code]", "", -1)
        // make into lines
        for _, code_line := range strings.Split(code_content, "\n") {
          //TODO: This also memearrows, should we?
          markup += memeline(code_line)
        }
        // close pre tag
        markup += "</pre>"
        // reset content buffer
        code_content = ""
      }
      // next line
      continue
    }
    // format line regularlly
    markup += memeline(line)
  }
  // flush the rest of an incomplete code tag
  for _, line := range strings.Split(code_content, "\n") {
    markup += memeline(line)
  }
  return 
}
