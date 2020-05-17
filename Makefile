PROJ := go-oled-controller
SRC  := $(wildcard *.go)

build: $(PROJ) $(PROJ).exe

$(PROJ): $(SRC)
	go build

$(PROJ).exe: $(SRC)
	# TODO: The cross-compiled binary crashes immediately.
	GOOS=windows GOARCH=386 \
		 CGO_ENABLED=1 CXX=i686-w64-mingw32-g++ CC=i686-w64-mingw32-gcc \
		 go build -a
