#
# models.py
#
# models for overchan primatives
#

__doc__ = """
models for overchan primatives
"""

from . import database
import hashlib

class Article:
    """
    overchan article
    """
    
    def __init__(self, postid):
        self.id = postid
        self.newsgroup = 'overchan.none'
        self.parent = None
        self.frontend = 'the.void.tld'
        self.name = 'Anonymous'
        self.key = None
        self.subject = None
        self.comment = ''
        
        
    def save(self):
        database.posts.insert().values([{
            'article_id' : self.id,
            'article_id_hash': hashlib.sha1(self.id).hexdigest(),
            'newsgroup':self.newsgroup,
            'parent': self.parent,
            'pubkey': self.key,
            'subject': self.subject,
            'comment': self.comment,
        }])
