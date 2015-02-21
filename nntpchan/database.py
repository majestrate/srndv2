#
# database.py
#
# sql database layer
#
__doc__ = """
database primatives
"""

from sqlalchemy import *
from sqlalchemy.dialects.postgres import UUID as GUID

from . import config

_meta = MetaData()

posts = Table('frontend_posts', _meta, 
            Column('article_id', Text),
            Column('id', GUID, primary_key=True),
            Column('newsgroup', Text, nullable=False),
            Column('parent', Text),
            Column('pubkey', Text),
            Column('subject', Text),
            Column('comment', Text))
              
files = Table('frotend_files', _meta,
            Column('file_id', GUID, primary_key=True),
            Column('filename', Text, nullable=False),
            Column('filepath', Text, nullable=False),
            Column('digest', Text, nullable=False),
            Column('parent_id', GUID, ForeignKey('frontend_posts.id'), nullable=False))



_engine = create_engine(config.get('db_url'))

_meta.bind = _engine
_meta.create_all()

def open():
    return _engine.connect()