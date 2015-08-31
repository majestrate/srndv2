//
// markup.go 
// memeposting markup parser
//
package srnd

import (
  "html"
  "strings"
)

func formatline(line string) (markup string) {
  line = strings.Trim(line, "\t\r\n ")
  // check for meme arrows
  if strings.HasPrefix(line, "&gt;") {
    markup += "<p><span class='memearrows'>"
    markup += line
    markup += "</span></p>"
  } else if strings.HasPrefix(line, "==") && strings.HasSuffix(line, "==") {
    // redtext
    markup += "<p><span class='redtext'>"
    markup += line[2:len(line)-2]
    markup += "</span></p>"
  } else {
    // regular line
    markup += "<p>"
    markup += line
    markup += "</p>"
  }
  return
}

// format lines inside a code tag
func formatcodeline(line string) (markup string) {
  markup += "<p>"
  markup += line
  markup += "</p>"
  return
}

func memeposting(src string) (markup string) {
  // escape
  src = html.EscapeString(src)

  found_tag := false
  tag_content := ""
  tag := ""
  // for each line...
  for _, line := range strings.Split(src, "\n") {
    // beginning of code tag ?
    if strings.Count(line, "[code]") > 0 {
      // yes there's a code tag
      found_tag = true
      tag = "code"
    } else if strings.Count(line, "[spoiler]") > 0 {
      // spoiler tag
      found_tag = true
      tag = "spoiler"
    } else if strings.Count(line, "[psy]") > 0 {
      // psy tag
      found_tag = true
      tag = "psy"
    }
    if found_tag {
      // collect content of tag
      tag_content += line + "\n"
      // end of our tag ?
      if strings.Count(line, "[/"+tag+"]") == 1 {
        // yah
        found_tag = false
        var tag_open, tag_close string
        if tag == "code" {
          tag_open = "<pre>"
          tag_close = "</pre>"
        } else if tag == "spoiler" {
          tag_open = "<span class='spoiler'>"
          tag_close = "</span>"
        } else if tag == "psy" {
          tag_open = "<span class='psy'>"
          tag_close = "</span>"          
        }
        markup += tag_open
        // remove open tag, only once so we can have a code tag verbatum inside
        tag_content = strings.Replace(tag_content, "["+tag+"]", "", 1)
        // remove all close tags, should only have 1
        tag_content = strings.Replace(tag_content, "[/"+tag+"]", "", -1)
        // make into lines
        for _, tag_line := range strings.Split(tag_content, "\n") {
          if tag == "code" {
            markup += formatcodeline(tag_line)
          } else {
            markup += formatline(tag_line)       
          }
        }
        // close pre tag
        markup += tag_close
        // reset content buffer
        tag_content = ""
      }
      // next line
      continue
    }
    // format line regularlly
    markup += formatline(line)
  }
  // flush the rest of an incomplete code tag
  for _, line := range strings.Split(tag_content, "\n") {
    markup += formatline(line)
  }
  return 
}
