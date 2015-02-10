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
        return False

class FileSystemArticleStore(BaseArticleStore):
    """
    article store that stores articles on the filesystem
    """

    def __init__(self, conf):
        super().__init__()
        self.base_dir = conf['base_dir']
        util.ensure_dir(self.base_dir)

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
