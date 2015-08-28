//
// templates.go
// template model interfaces
//
package srnd

import (
  "fmt"
  "io"
  "log"
  "sort"
)

// base model type
type BaseModel interface {

  // site url prefix
  Prefix() string

  // render to a writer
  RenderTo(wr io.Writer) error

}


// for attachments
type AttachmentModel interface {

  BaseModel
  
  Thumbnail() string
  Source() string
  Filename() string
  
}

// for individual posts
type PostModel interface {

  BaseModel

  CSSClass() string
  
  MessageID() string
  PostHash() string
  ShortHash() string
  PostURL() string
  Frontend() string
  Subject() string
  Name() string
  Date() string
  OP() bool
  Attachments() []AttachmentModel
  Board() string
  Sage() bool
  Pubkey() string
  Reference() string
  
  RenderBody() string
  RenderPost() string

  // truncate body to a certain size
  // return copy
  Truncate(amount int) PostModel
  
}

// interface for models that have a navbar
type NavbarModel interface {

  Navbar() string

}

// for threads
type ThreadModel interface {

  BaseModel
  NavbarModel
  
  OP() PostModel
  Replies() []PostModel
  Board() string
  BoardURL() string
}

// board interface
type BoardModel interface {

  BaseModel
  NavbarModel
  
  Frontend() string
  Name() string
  Threads() []ThreadModel
}

type LinkModel interface {

  Text() string
  LinkURL() string
}


func genUkko(prefix, outfile string, database Database) {
  log.Println("regen ukko")
  // get the last 5 bumped threads
  roots := database.GetLastBumpedThreads("", 5)
  var threads []ThreadModel
  for _, rootpost := range roots {
    // for each root post
    // get the last 5 posts
    post := database.GetPostModel(prefix, rootpost)
    if post == nil {
      log.Println("failed to get root post", rootpost)
      return
    }
    // TODO: hardcoded value
    posts := []PostModel{post.Truncate(512)}
    if database.ThreadHasReplies(rootpost) {
      repls := database.GetThreadReplyPostModels(prefix, rootpost, 5)
      if repls == nil {
        log.Println("failed to get replies for", rootpost)
        return
      }
      for _, repl := range repls {
        // truncate reply size
        posts = append(posts, repl.Truncate(512))
      }
    }
    threads = append(threads, thread{
      prefix: prefix,
      posts: posts,
    })
  }
  wr, err := OpenFileWriter(outfile)
  if err == nil {
    io.WriteString(wr, renderUkko(prefix, threads))
    wr.Close()
  } else {
    log.Println("error generating ukko", err)
  }
}

func genFrontPage(top_count int, frontend_name, outfile string, db Database) {
  log.Println("regen front page")
  // the graph for the front page
  var frontpage_graph boardPageRows

  // for each group
  groups := db.GetAllNewsgroups()
  for idx, group := range groups {
    if idx >= top_count {
      break
    }
    // posts per hour
    hour := db.CountPostsInGroup(group, 3600)
    // posts per day
    day := db.CountPostsInGroup(group, 86400)
    // posts total
    all := db.CountPostsInGroup(group, 0)
    frontpage_graph = append(frontpage_graph, boardPageRow{
      All: all,
      Day: day,
      Hour: hour,
      Board: group,
    })
  }
  wr, err := OpenFileWriter(outfile)
  if err != nil {
    log.Println("cannot render front page", err)
    return
  }

  param := make(map[string]interface{})
  sort.Sort(frontpage_graph)
  param["graph"] = frontpage_graph
  param["frontend"] = frontend_name
  param["totalposts"] = db.ArticleCount()
  _, err = io.WriteString(wr, renderTemplate("frontpage.mustache", param))
  if err != nil {
    log.Println("error writing front page", err)
  }
  wr.Close() 
}

func genThread(rootMessageID, prefix, outfile string, database Database) {
  // get the root post
  var posts []PostModel
  op := database.GetPostModel(prefix, rootMessageID)
  // get op if null get placeholder
  if op == nil {
    log.Println("no root post for", rootMessageID)
    repls := database.GetThreadReplyPostModels(prefix, rootMessageID, 0)
    if repls == nil {
      // wtf do we do? idk
      log.Println("no replies for? wtf", rootMessageID)
      return
    } else {
      posts = append(posts, post{
        prefix: prefix,
        subject: "[no root post yet]",
        message: "this is a placeholder for "+rootMessageID,
        message_id: rootMessageID,
        name: "???",
        path: "missing.frontend",
        board: repls[0].Board(),
      })
      posts = append(posts, repls...)
      op = posts[0]
    }
  } else {
    posts = append(posts, op)
    if database.ThreadHasReplies(rootMessageID) {
    repls := database.GetThreadReplyPostModels(prefix, rootMessageID, 0)
    if repls == nil {
      log.Println("failed to regen thread, replies was nil for op", rootMessageID)
      return
    }
    posts = append(posts, repls...)
  }

  }
  // the link that points back to the board index
  back_link := linkModel{
    text: "back to board index",
    link: fmt.Sprintf("%s%s-0.html", prefix, op.Board()),
  }
  // the links for this thread
  links := []LinkModel{back_link}
  // make thread model
  thread := thread{
    prefix: prefix,
    links: links,
    posts: posts,
  }
  // get filename for thread
  // open writer for file
  wr, err := OpenFileWriter(outfile)
  if err != nil {
    log.Println(err)
    return
  }
  // render the thread
  err = thread.RenderTo(wr)
  wr.Close()
  if err == nil {
  } else {
    log.Printf("failed to render %s", err)
  }
}


// regenerate a board page for newsgroup
func genBoardPage(outfile, prefix, frontend, newsgroup string, pageno int, database Database) {
  var err error
  var perpage int
  perpage, err = database.GetThreadsPerPage(newsgroup)
  if err != nil {
    log.Println("board regen fallback to default threads per page because", err)
    // fallback
    perpage = 10
  }
  board_page := database.GetGroupForPage(prefix, frontend, newsgroup, pageno, perpage)
  if board_page == nil {
    log.Println("failed to regen board", newsgroup)
    return
  }
  wr, err := OpenFileWriter(outfile)
  if err == nil {
    err = board_page.RenderTo(wr)
    wr.Close()
    if err != nil {
      log.Println("did not write board page",outfile, err)
    }
  } else {
    log.Println("cannot open", outfile, err)
  }
  // clear reference
  board_page = nil
}
