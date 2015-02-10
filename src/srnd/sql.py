#
# sql.py
#

from sqlalchemy import *

_metadata = MetaData()

newsgroups = Table("newsgroups", _metadata,
                   Column("updated", Integer),
                   Column("name", Text, unique=True, primary_key=True))

articles = Table("articles", _metadata,
                 Column("id", Text, primary_key=True),
                 Column("message", Text),
                 Column("posted_at", Integer),
                 Column("name", Text),
                 Column("subject", Text),
                 Column("pubkey", Text),
                 Column("sig", Text),
                 Column("email", Text),
                 Column("references", Text),
                 Column("filename", Text),
                 Column("filepath", Text),
                 Column("thumbpath", Text),
                 Column("imagehash", Text),
                 Column("posthash", Text))
                     

class SQL:
    """
    generic sql driver
    used to do sql queries to backend
    wraps sql alchemy
    """

    def __init__(self, dbconf):
        self.engine = create_engine(dbconf['url'])

    def connect(self):
        self.con = self.engine.connect()


# 
# initialize database
#
from . import config
sql = SQL(config.load_config()['database'])
_metadata.create_all(sql.engine)

