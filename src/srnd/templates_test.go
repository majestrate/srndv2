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
			template.genBoardPage(true, "prefix", "test", "overchan.overchan", 0, "boardpage.html", db)
		}
	})
}

func BenchmarkRenderThread(b *testing.B) {
	db := NewDatabase("postgres", "srnd", "/var/run/postgresql", "", "", "")
	db.CreateTables()
	defer db.Close()
	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			template.genThread(true, ArticleEntry{"<ab0ff1453596701@web.oniichan.onion>", "overchan.technology"}, "prefix", "frontend", "thread.html", db)
		}
	})
}
