package srnd

import (
	"testing"
)


func BenchmarkRenderBoardPage(b *testing.B) {
	db := NewDatabase("postgres", "srnd", "/var/run/postgresql", "", "", "")
	db.CreateTables()
	defer db.Close()
	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			template.genBoardPage(true, "prefix", "test", "overchan.random", 0, "boardpage.html", db)
		}
	})
}

func BenchmarkRenderThread(b *testing.B) {
	db := NewDatabase("postgres", "srnd", "/var/run/postgresql", "", "", "")
	db.CreateTables()
	defer db.Close()
	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			template.genThread(true, ArticleEntry{"<25ae01453624341@web.ucavviu7wl6azuw7.onion>", "overchan.random"}, "prefix", "frontend", "thread.html", db)
		}
	})
}
