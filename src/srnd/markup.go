//
// markup.go
// memeposting markup parser
//
package srnd

import (
	"github.com/mvdan/xurls"
	"html"
	"regexp"
	"strings"
)

// copypasted from https://stackoverflow.com/questions/161738/what-is-the-best-regular-expression-to-check-if-a-string-is-a-valid-url
// var re_external_link = regexp.MustCompile(`((?:(?:https?|ftp):\/\/)(?:\S+(?::\S*)?@)?(?:(?!(?:10|127)(?:\.\d{1,3}){3})(?!(?:169\.254|192\.168)(?:\.\d{1,3}){2})(?!172\.(?:1[6-9]|2\d|3[0-1])(?:\.\d{1,3}){2})(?:[1-9]\d?|1\d\d|2[01]\d|22[0-3])(?:\.(?:1?\d{1,2}|2[0-4]\d|25[0-5])){2}(?:\.(?:[1-9]\d?|1\d\d|2[0-4]\d|25[0-4]))|(?:(?:[a-z\u00a1-\uffff0-9]-*)*[a-z\u00a1-\uffff0-9]+)(?:\.(?:[a-z\u00a1-\uffff0-9]-*)*[a-z\u00a1-\uffff0-9]+)*(?:\.(?:[a-z\u00a1-\uffff]{2,}))\.?)(?::\d{2,5})?(?:[/?#]\S*)?)`);
var re_external_link = xurls.Strict
var re_backlink = regexp.MustCompile(`>> ?([0-9a-f]+)`)

// parse backlink
func backlink(word string) (markup string) {
	re := regexp.MustCompile(`>> ?([0-9a-f]+)`)
	link := re.FindString(word)
	if len(link) > 2 {
		link = strings.Trim(link[2:], " ")
		if len(link) > 2 {
			url := template.findLink(link)
			if len(url) == 0 {
				return "<span class='memearrows'>&gt;&gt;" + link + "</span>"
			}
			// backlink exists
			return `<a href="` + url + `">&gt;&gt;` + link + "</a>"
		} else {
			return html.EscapeString(word)
		}
	}
	return html.EscapeString(word)
}

func formatline(line string) (markup string) {
	line = strings.Trim(line, "\t\r\n ")
	if len(line) > 0 {
		if strings.HasPrefix(line, ">") && !(strings.HasPrefix(line, ">>") && re_backlink.MatchString(strings.Split(line, " ")[0])) {
			// le ebin meme arrows
			markup += "<span class='memearrows'>"
			markup += html.EscapeString(line)
			markup += "</span>"
		} else if strings.HasPrefix(line, "==") && strings.HasSuffix(line, "==") {
			// redtext
			markup += "<span class='redtext'>"
			markup += html.EscapeString(line[2 : len(line)-2])
			markup += "</span>"
		} else {
			// regular line
			// for each word
			for _, word := range strings.Split(line, " ") {
				// check for backlink
				if re_backlink.MatchString(word) {
					markup += backlink(word)
				} else {
					// linkify as needed
					word = html.EscapeString(word)
					markup += re_external_link.ReplaceAllString(word, `<a href="$1">$1</a>`)
				}
				markup += " "
			}
		}
	}
	markup += "<br />"
	return
}

func memeposting(src string) (markup string) {
	for _, line := range strings.Split(src, "\n") {
		markup += formatline(line)
	}
	return
}
