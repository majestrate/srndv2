#!/usr/bin/env python
#
# srndv2 reference frontend
#
from . import overchan
import logging
from tornado import ioloop, web

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--bind', type=int, default=8383)
    ap.add_argument('--socket', type=str, required=True)
    ap.add_argument('--name', type=str, default='nntpchan')
    ap.add_argument('--debug', action='store_const', const=True, default=False)
    args = ap.parse_args()
    
    loglvl = args.debug and logging.DEBUG or logging.INFO
    logging.basicConfig(level=loglvl, format="%(created)d %(levelname)s %(name)s %(message)s")

    loop = ioloop.IOLoop.instance()

    frontend = overchan.Frontend(loop, args.name)

    context = {'srndapi': frontend}
    
    app = web.Application([
        (r"/mod", overchan.ModHandler, context),
        (r"/post", overchan.PostHandler, context),
    ])

    frontend.bind()
    frontend.connect(args.socket)
    app.listen(args.bind)
    loop.start()
    
if __name__ == '__main__':
    main()
