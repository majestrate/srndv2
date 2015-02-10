#
# storage.py
#
import contextlib
import os

from . import util
from . import sql

class BaseArticleStore:
    """
    base class for article storage
    stores news articles
    """

    def has_article(self, article_id):
        """
        return true if we have an article
        """
        return False

    @contextlib.contextmanager
    def open_article(self, article_id):
        """
        open an article so someone can do stuff
        """
        yield

    def article_banned(self, article_id):
        """
        return true if this article is banned
        """
        return False

    def has_group(self, newsgroup):
        """
        return true if we carry this newsgroup
        """
        return False
        
    def group_banned(self, newsgroup):
        """
        return true if this news group is locally banned
        """
        return False

    def get_group_info(self, newsgroup):
        """
        return tuple number, min_posts, max_posts
        """
        return 0, 0, 0
        

class FileSystemArticleStore(BaseArticleStore):
    """
    article store that stores articles on the filesystem
    """

    def __init__(self, conf):
        super().__init__()
        self.base_dir = conf['base_dir']
        util.ensure_dir(self.base_dir)
        self.db = sql.SQL()
        self.db.connect()

    def has_group(self, newsgroup):
        res = self.db.connection.execute(
            sql.select([sql.func.count(sql.newsgroups.c.name)]).where(
                sql.newsgroups.c.name == newsgroup)
            ).scalar()
        return res != 0

    @contextlib.contextmanager
    def open_article(self, article_id, read=False):
        assert util.is_valid_article_id(article_id)
        fd = open(os.path.join(self.base_dir, article_id) ,'wb')
        yield fd
        fd.close()

    def has_article(self, article_id):
        if util.is_valid_article_id(article_id):
            return os.path.exists(os.path.join(self.base_dir, article_id))
        return True

    def get_group_info(self, group):
        res = self.db.connection.execute(
            sql.select([sql.func.count(sql.articles.c.message_id)]).where(
                sql.articles.c.newsgroup == group)).scalar()
        return res, 0, 1000000

    def __del__(self):
        self.db.close()
