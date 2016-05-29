package frontend_cache

import (
	"github.com/majestrate/srndv2/lib/model"
)

type MarkupGenerator interface {
	GenerateFrontPage() string
	GenerateHistory() string
	GenerateBoards() string
	GenerateUkko() string
	GenerateThread(model.ArticleEntry) string
	GenerateCatalog(string) string
	GenerateBoardPage(string, int) string
}
