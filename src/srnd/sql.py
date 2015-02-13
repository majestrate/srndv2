#
# sql.py
#

from . import config
from sqlalchemy import *

_metadata = MetaData()

users = Table("users", _metadata,
              Column("name", String, unique=True),
              Column("passwd", String),
              Column("uid", Integer, primary_key=True))

newsgroups = Table("newsgroups", _metadata,
                   Column("updated", DateTime, default=func.now()),
                   Column("article_count", Integer, default=0),
                   Column("name", Text, unique=True, primary_key=True))

article_posts = Table("article_posts", _metadata,
                      Column("post_id", Integer),
                      Column("newsgroup", String),
                      Column("article_id", String))

articles = Table("articles", _metadata,
                 Column("message_id", Text, primary_key=True),
                 Column("message", Text),
                 Column("posted_at", Integer),
                 Column("name", Text),
                 Column("subject", Text),
                 Column("pubkey", Text),
                 Column("sig", Text),
                 Column("email", Text),
                 Column("references", Text),
                 Column("filename", Text),
                 Column("imagehash", Text),
                 Column("posthash", Text))
                     

class SQL:
    """
    generic sql driver
    used to do sql queries to backend
    wraps sql alchemy
    """

    def __init__(self, dbconf=None):
        if dbconf is None:
            dbconf = config.load_config()['database']
        self.engine = create_engine(dbconf['url'])
        self.connection = None

    def connect(self):
        if self.connection:
            return
        self.connection = self.engine.connect()

    def close(self):
        if self.connection:
            self.connection.close()
        self.connection = None

    def __del__(self):
        self.close()
        
# 
# initialize database
#
def create(dbconf=None):
    if dbconf is None:
        dbconf = config.load_config()['database']
    sql = SQL(dbconf)
    _metadata.create_all(sql.engine)
    return sql


db = SQL()
db.connect()
