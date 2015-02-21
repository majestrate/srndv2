#!/usr/bin/env python
#
# srndv2 overchan+postman plugin
#
from . import overchan
from . import srndapi

import logging

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--bind', type=str, default='127.0.0.1:8383')
    ap.add_argument('--name', type=str, default='nntpchan')
    ap.add_argument('--debug', action='store_const', const=True, default=False)
    args = ap.parse_args()
    
    loglvl = args.debug and logging.DEBUG or logging.INFO
    logging.basicConfig(level=loglvl)
    overchan.run()
    
if __name__ == '__main__':
    main()