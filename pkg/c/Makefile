# Lux Consensus C Library Makefile
CC = gcc
AR = ar
CFLAGS = -Wall -Wextra -O3 -fPIC -pthread -I./include
LDFLAGS = -shared -pthread

# Platform detection
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
    DYLIB_EXT = dylib
    LDFLAGS += -dynamiclib -install_name @rpath/libluxconsensus.dylib
else
    DYLIB_EXT = so
    LDFLAGS += -Wl,-soname,libluxconsensus.so
endif

# Source files
SOURCES = src/consensus_engine.c
OBJECTS = $(SOURCES:.c=.o)

# Target files
STATIC_LIB = lib/libluxconsensus.a
SHARED_LIB = lib/libluxconsensus.$(DYLIB_EXT)

# Build targets
all: $(STATIC_LIB) $(SHARED_LIB)

$(STATIC_LIB): $(OBJECTS) | lib
	$(AR) rcs $@ $^

$(SHARED_LIB): $(OBJECTS) | lib
	$(CC) $(LDFLAGS) -o $@ $^

%.o: %.c
	$(CC) $(CFLAGS) -c $< -o $@

lib:
	mkdir -p lib

clean:
	rm -f $(OBJECTS)
	rm -f $(STATIC_LIB) $(SHARED_LIB)
	rm -rf lib

install: $(STATIC_LIB) $(SHARED_LIB)
	install -d /usr/local/lib
	install -d /usr/local/include
	install -m 644 $(STATIC_LIB) /usr/local/lib/
	install -m 755 $(SHARED_LIB) /usr/local/lib/
	install -m 644 include/lux_consensus.h /usr/local/include/

test: $(SHARED_LIB)
	$(CC) -o test/test_consensus test/test_consensus.c -L./lib -lluxconsensus -pthread
	./test/test_consensus

.PHONY: all clean install test