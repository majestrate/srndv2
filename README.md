# SRNDv2 #

Some Random News Daemon version 2

overchan nntp daemon

status: in dev

donate: bitcoin 15yuMzuueV8y5vPQQ39ZqQVz5Ey98DNrjE
	

## requirements ##

* go 1.4 or higher

## building

    # set gopath if it's not already set
    export GOPATH=$HOME/go
    mkdir -p $GOPATH

    # get source code
    go get github.com/majestrate/srndv2
    cd $GOPATH/src/github.com/majestrate/srndv2

    # get git submodules
    git submodule init
    git submodule update

    # clean any previous builds
    ./clean
    # build everything
    # this builds libsodium too so it could take a bit
    ./build

## rebuilding

    ./build core


## initial run

    # this will generate base config files if they aren't present
    ./run


## configuration

    see config.md

## Notes

When receiving many articles (i.e. durring initial sync with a network with over
9000 posts (or so) disable outfeeds as there is an unresolved race condition
