MAN	= xd.1
CATMAN	= xd.cat

all: $(CATMAN)
	go build

$(CATMAN): $(MAN)
	nroff -c -mdoc $< >$@

clean:
	-rm -rf $(CATMAN)
	go clean

.PHONY: all clean
