#
# sql.py
#

from . import config
from sqlalchemy import *

_metadata = MetaData()

newsgroups = Table("newsgroups", _metadata,
                   Column("updated", Integer),
                   Column("first", Integer),
                   Column("last", Integer),
                   Column("name", Text, unique=True, primary_key=True))

articles = Table("articles", _metadata,
                 Column("newsgroup", Text),
                 Column("post_id", Integer, autoincrement=1, primary_key=True),
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
