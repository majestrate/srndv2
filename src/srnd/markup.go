//
// markup.go 
// memeposting markup parser
//
package srnd

import (
  "bytes"
  "fmt"
  "html"
  "strings"

)

func memeposting(src string) string {
  var buff bytes.Buffer
  // split into lines
  lines := strings.Split(src, "\n")
  // for each line

  
  
  for _, line := range lines {
    // trim whitespaces from line
    line = strings.Trim(line, " \r")

    // escape it nigga!
    esc_line := html.EscapeString(line)
    
    // write start of line
    buff.WriteString("<p>")

    if strings.HasPrefix(line, ">") {
      buff.WriteString(fmt.Sprintf("<span class='memearrows'>%s<span>", esc_line))
    } else {
      buff.WriteString(esc_line)
    }
    // write end of line
    buff.WriteString("</p>")
  }
  return buff.String()
}
