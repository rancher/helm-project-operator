TARGETS := $(shell ls scripts)

$(TARGETS): 
	./scripts/$@

PHONY: $(TARGETS)
