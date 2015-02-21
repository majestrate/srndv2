#
# models.py
#
# models for overchan primatives
#

__doc__ = """
models for overchan primatives
"""

class Article:
    """
    overchan article
    """
    
    def __init__(self, postid):
        self.id = postid
        self.newsgroup = 'overchan.none'
        self.op = None
        self.thread = None
        self.frontend = 'the.void.tld'
        self.name = 'Anonymous'
        self.key = None
        self.subject = None
        self.comment = None
        self.files = list()
        
        
        