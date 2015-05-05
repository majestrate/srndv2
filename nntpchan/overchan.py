#
# overchan.py
#
# frontend reference implementation
#

__doc__ = """
frontend reference implementation
"""
from . import database
from . import models
from . import srndapi
from . import util

from tornado import web

import logging
import os
import base64


class WebHandler(web.RequestHandler):
    def initialize(self, srndapi):
        self.api = srndapi


class IndexHandler(WebHandler):
    def get(self):
        self.write("""
        <html>
        <body>
        <form action="/post" method="POST">
        <input name="newsgroup" type="hidden" value="overchan.test" />
        <input name="message" type="text" />
        <input type="submit" value="post" />
        </form>
        </body>
        </html>
        """)

class NewsgroupHandler(WebHandler):

    def get(self, newsgroup):
        """
        """

        
class ThreadHandler(WebHandler):

    def get(self, thread):
        """
        """
        
class PostHandler(WebHandler):

    def post(self):
        """
        """
        newsgroup = self.get_argument("newsgroup")
        parent = self.get_argument("parent", default="")
        name = self.get_argument("name", default="Anonymous")
        email =  self.get_argument("email", default="anon@nowhere.tld")
        subject = self.get_argument("subject", default="None")
        message = self.get_argument("message")
        msg = self.api.post(newsgroup, parent, name, email, subject, message)
        self.write(msg)

class Frontend(srndapi.SRNdAPI):
    """
    srndv2 overchan+postman reference implementation
    """
    def __init__(self, loop, name):
        srndapi.SRNdAPI.__init__(self, loop, name+".sock", name)
        self.name = name
        self.sql = database.open()
    
    def got(self, obj):
        """
        we got an incoming object
        """
        if obj["Please"] == "post":
            self.got_post(obj)
            
    def put_file(self, file_obj):
        """
        put a file onto the disk
        """
        fname = os.path.join("img", "{}{}".format(util.now(), file_obj["Extension"]))
        self.log.info("putfile {}".format(fname))
        d = file_obj["Data"]
        d = base64.b64decode(d)
        with open(fname, 'w') as f:
            f.write(d)
        
    def got_post(self, obj):
        """
        we got a post
        """
        self.log.info("we got a post")
        if obj['Attachments']:
            for f in obj['Attachments']:
                self.put_file(f)
        postid = obj['MessageID']
        post = models.Article(postid)
        for attr in obj.keys():
            if attr in ('Please', 'OP', 'Attachments'):
                continue
            val = obj[attr]
            if val:
                if isinstance(val, bool) or isinstance(val, int):
                    setattr(post, attr, val)
                elif len(val) > 0:
                    setattr(post, attr, val)

    def find_thread(self, msgid):
        """
        find a thread with the root post given the hashed post id
        """
        
        
    def sync(self, newsgroups=None):
        if newsgroups is None:
            newsgroups = list()
            self.log.info('sync all groups')
        else:
            self.log.info('synching {} groups'.format(len(newsgroups)))
        self.please('sync', newsgroups=newsgroups)
        
    def genID(self):
        return '<{}.{}@{}>'.format(util.random_string(10), util.now(), self.name)
        
    def post(self, newsgroup, parent, name, email, subject, comment, key=None):
        """
        handle a post from flask
        """
        obj = dict()
        if not newsgroup.startswith('overchan.'):
            return 'invalid newsgroup'
        obj['MessageID'] = self.genID()
        obj['Newsgroup'] = newsgroup
        obj['Reference'] = parent
        obj['Name'] = name
        obj['Email'] = email
        obj['Subject'] = subject
        obj['Comment'] = comment
        obj['Key'] = key
        obj['Posted'] = util.now()
        self.please('post', **obj)
        return "posted"
        
    def register_api(self, api):
        self.log.info('registering with api...')

    def has_thread(self, thread_id):
        pass
