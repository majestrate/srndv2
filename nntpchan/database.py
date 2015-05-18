#
# database.py
#
# sql database layer
#
__doc__ = """
database primatives
"""

from sqlalchemy import *

from . import config

_engine = create_engine(config.get('db_url'))

def open():
    return _engine.connect()
