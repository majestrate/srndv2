package srnd

import (
	"testing"
)

func makeBenchmarkDB() Database {
	//return NewDatabase("postgres", "srnd", "/var/run/postgresql", "", "", "")
	return NewDatabase("redis", "single", "localhost", "6379", "", "")
}

func BenchmarkRenderBoardPage(b *testing.B) {
	db := makeBenchmarkDB()
	db.CreateTables()
	defer db.Close()
	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			template.genBoardPage(true, "prefix", "test", "overchan.random", 0, "boardpage.html", db)
		}
	})
}

func BenchmarkRenderThread(b *testing.B) {
	db := makeBenchmarkDB()
	db.CreateTables()
	defer db.Close()
	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			template.genThread(true, ArticleEntry{"<c49be1451427261@nntp.nsfl.tk>", "overchan.random"}, "prefix", "frontend", "thread.html", db)
		}
	})
}
