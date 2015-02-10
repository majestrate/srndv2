#
# test_sql.py
#
from sqlalchemy import *
from . import sql

import pytest

def test_connect():
    db = sql.SQL({'url':'sqlite:///:memory:'})
    db.connect()

def test_insert_article():
    dbconf = {'url':'sqlite:///:memory:'}
    message_id = '<test@ayyy.lmao>'
    db = sql.create(dbconf)

    db.connect()

    # INSERT INTO articles VALUES ...
    db.connection.execute(
        sql.articles.insert(), {
            'message_id': message_id,
            'message': 'ayyylmao'
        })
    
    # SELECT COUNT(message_id) FROM articles WHERE message_id = ?
    res = db.connection.execute(sql.select(
        [sql.func.count(sql.articles.c.message_id)]).where(
            sql.articles.c.message_id == message_id)).scalar()

    assert res == 1
