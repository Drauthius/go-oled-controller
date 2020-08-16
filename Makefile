PROJ := go-oled-controller
SRC  := $(wildcard *.go)

CROSS_COMPILE_PREFIX ?= x86_64-w64-mingw32-

build: $(PROJ) $(PROJ).exe

$(PROJ): $(SRC)
	go build -o $@

# Cross-compile to Windows.
# Note: 64 bit is required for NVIDIA library (NVML).
$(PROJ).exe: $(SRC)
	GOOS=windows GOARCH=amd64 \
		 CGO_ENABLED=1 CC=$(CROSS_COMPILE_PREFIX)gcc CXX=$(CROSS_COMPILE_PREFIX)g++ \
		 go build -o $@ -a
