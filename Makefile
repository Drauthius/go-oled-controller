PROJ := go-oled-controller
SRC  := $(wildcard *.go)

CROSS_COMPILE_PREFIX ?= x86_64-w64-mingw32-

# This uncommented out section doesn't work. The program crashes immediately on Windows.
#
# Path to the dlfcn-win32 library (Download and build https://github.com/dlfcn-win32/dlfcn-win32)
#WIN64_LIBDL ?= /usr/x86_64-w64-mingw32/usr/lib/libdl.a
# Path to the NVML library (Download CUDA Toolkit and get it from the extract step)
#WIN64_NVML ?= /usr/x86_64-w64-mingw32/usr/lib/nvml.lib
#CGO_LDFLAGS="$(WIN64_LIBDL) $(WIN64_NVML)"

build: $(PROJ) $(PROJ).exe

$(PROJ): $(SRC)
	go build

# Cross-compile to Windows.
# 64bit is required for NVIDIA library (NVML).
$(PROJ).exe: $(SRC)
	GOOS=windows GOARCH=amd64 \
		 CGO_ENABLED=1 CC=$(CROSS_COMPILE_PREFIX)gcc CXX=$(CROSS_COMPILE_PREFIX)g++ \
		 go build -a
