#!/usr/bin/env python3

import logging
import os
import subprocess

logger = logging.getLogger(__name__)

def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument('--studio', default=None, help='path to ballotstudio')
    ap.add_argument('-v', '--verbose', default=False, action='store_true')
    args = ap.parse_args()
    if args.verbose:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)
    pwd = os.path.abspath(os.getcwd())
    studiodir = args.studio
    if not studiodir:
        studiodir = os.path.abspath(os.path.join(pwd, '..', 'ballotstudio'))
        logger.debug('guessing studiodir %r', studiodir)
    with open('nginx.conf.in') as fin:
        raw = fin.read()
    x = raw.replace('@@ROOTDIR@@', pwd)
    x = x.replace('@@STUDIODIR@@', studiodir)
    os.makedirs(os.path.join(pwd, 'nginx', 'log'), exist_ok=True)
    os.makedirs(os.path.join(pwd, 'nginx', 'cache'), exist_ok=True)
    os.makedirs(os.path.join(pwd, 'nginx', 'ctmp'), exist_ok=True)
    os.makedirs(os.path.join(pwd, 'nginx', 'tmp'), exist_ok=True)
    confpath = os.path.join(pwd, 'nginx','nginx.conf')
    with open(confpath, 'wt') as fout:
        fout.write(x)
    os.execvp('nginx', ['nginx', '-c', confpath, '-p', os.path.join(pwd, 'nginx')])

if __name__ == '__main__':
    main()
