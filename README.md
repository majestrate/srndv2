# SRNDv2 #

Some Random News Daemon version 2

overchan nntp daemon

status: in dev

donate: bitcoin 15yuMzuueV8y5vPQQ39ZqQVz5Ey98DNrjE
	


## buiding ##

    # requires python 3.4 or higher
    pyvenv venv
	venv/bin/pip install -r requirements.txt

## testing ##

    venv/bin/pip install pytest
	cd src
	../venv/bin/py.test
	
## running ##

    cd src
    ../venv/bin/python -m srnd

