all:
	go build -o SRNd src/*.go
clean:
	rm -f SRNd *.sock *~
