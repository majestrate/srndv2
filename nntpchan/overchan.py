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

import logging
import flask

class Frontend(srndapi.SRNdAPI):
    """
    srndv2 overchan+postman reference implementation
    """
    def __init__(self):
        name = 'overchan.srndv2.tld'
        srndapi.SRNdAPI.__init__(self, name+'.sock', name)
        self.sql = database.open()
        
    def got(self, obj):
        """
        we got an incoming object
        """
        self.log.info("got {}".format(obj["Please"]))
        if obj["Please"] == "post":
            self.got_post(obj)
            
    def put_file(self, file_obj):
        """
        put a file onto the disk
        """
        self.log.info("putfile")
            
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
        self.log.info("loading...")
        for attr in obj.keys():
            if attr in ('Please', 'OP', 'Attachments'):
                continue
            val = obj[attr]
            if val:
                if isinstance(val, bool):
                    setattr(post, attr, val)
                elif len(val) > 0:
                    setattr(post, attr, val)
        self.log.info("saved")
            
            
        
    def sync(self, newsgroups=None):
        if newsgroups is None:
            newsgroups = list()
            self.log.info('sync all groups')
        else:
            self.log.info('synching {} groups'.format(len(newsgroups)))
        self.please('sync', newsgroups=newsgroups)
        
    def genID(self):
        return '<{}.{}@{}>'.format(util.random_string(), util.now(), self.name)
        
    def post(self, newsgroup, parent, name, email, subject, comment, key=None):
        """
        handle a post from flask
        """
        obj = dict()
        if not newsgroup.startswith('overchan.'):
            return 'invalid newsgroup'
        obj['id'] = self.genID()
        obj['newsgroup'] = newsgroup
        obj['parent'] = parent
        obj['name'] = name
        obj['email'] = email
        obj['subject'] = subject
        obj['comment'] = comment
        obj['key'] = key
        self.please('post', obj)
        
    def register_api(self, api):
        self.log.info('registering with api...')

    def has_thread(self, thread_id):
        pass

frontend = Frontend()

app = flask.Flask(__name__)

@app.route('/')
def handle_index():
    return flask.render_template('index.html', page_title=frontend.name)
    
@app.route('/thread/<string:thread_id>')
def handle_thread(thread_id):
    if not frontend.has_thread(thread_id):
        flask.abort(404)
    posts = frontend.get_thread(thread_id)
    if len(posts) > 1:
        replies = posts[1:]
    else:
        replies = list()
    return flask.render_template('thread.html', op=posts[0], replies=replies)
    
def run():
    frontend.start()
    frontend.connect()
    frontend.sync()
    app.run()