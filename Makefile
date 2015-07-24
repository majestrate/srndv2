all: clean build

build:
	go build -o srnd main.go
clean:
	rm -f srnd
