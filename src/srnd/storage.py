#
# storage.py
#
import contextlib
import logging
import os
import time

from . import util
from . import sql

class FileSystemArticleStore:
    """
    article store that stores articles on the filesystem
    """

    def __init__(self, daemon, conf):
        self.daemon = daemon
        self.base_dir = conf['base_dir']
        util.ensure_dir(self.base_dir)
        self.db = sql.SQL()
        self.db.connect()
        self.log = logging.getLogger('fs-storage')

    def yield_all_articles(self):
        """
        yield every article we have
        """
        for f in os.listdir(self.base_dir):
            groups = list()
            for group in self.get_groups_for_article(f):
                groups.append(group)
            yield tuple([f, groups])

    def get_groups_for_article(self, article_id):
        query = self.db.connection.execute(
            sql.select([sql.article_posts.c.newsgroup]).where(
                sql.article_posts.c.article_id == article_id))
        for res in query.fetchall():
            yield res[0]

    def check_user_login(self, user, passwd):
        """
        todo: hash passwords D:
        """
        res = self.db.connection.execute(
            sql.select([sql.users.c.passwd]).where(
                sql.users.c.name == user)).fetchone()
        if res:
            return res[0] == passwd
        return False
        
    def save_message(self, msg):
        self.log.info('save message {}'.format(msg.message_id))
        for group in msg.groups:
            if not self.has_group(group):
                self.db.connection.execute(sql.newsgroups.insert(),{'name': group})
        msg.save(self.db.connection)

    def get_all_groups(self):
        for res in self.db.connection.execute(
                sql.select([
                    sql.newsgroups.c.name])):
            yield res[0]

    def has_group(self, newsgroup):
        res = self.db.connection.execute(
            sql.select([sql.func.count(sql.newsgroups.c.name)]).where(
                sql.newsgroups.c.name == newsgroup)
            ).scalar()
        return res != 0


    @contextlib.contextmanager
    def open_article(self, article_id, read=False):
        assert util.is_valid_article_id(article_id)
        mode = read and 'r' or 'wb'
        fd = open(os.path.join(self.base_dir, article_id) ,mode)
        yield fd
        fd.close()

    def has_article(self, article_id):
        assert util.is_valid_article_id(article_id)
        return os.path.exists(os.path.join(self.base_dir, article_id))
        
    def delete_article(self, article_id):
        if self.has_article(article_id):
            os.unlink(os.path.join(self.base_dir, article_id))
        
    def get_group_info(self, group):
        self.log.info('get group info for {}'.format(group))
        # TODO optimize
        count = self.db.connection.execute(
            sql.select([sql.newsgroups.c.article_count]).where(
                sql.newsgroups.c.name == group)).fetchone()[0]
        if count > 0:
            return count, 1, count
        else:
            return 0, 0, 0
            
    def __del__(self):
        self.db.close()
