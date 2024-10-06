TARGETS := $(shell ls scripts)
LOCAL_TARGETS := $(addprefix local-,$(TARGETS))

.dapper:
	@echo Downloading dapper
	@curl -sL https://releases.rancher.com/dapper/latest/dapper-$$(uname -s)-$$(uname -m) > .dapper.tmp
	@@chmod +x .dapper.tmp
	@./.dapper.tmp -v
	@mv .dapper.tmp .dapper

# Default behavior for targets without dapper
$(TARGETS):
	@scripts/$@

# Behavior for targets prefixed with "local-" using dapper
$(LOCAL_TARGETS): local-%: .dapper
	./.dapper $(@:local-%=%)

.DEFAULT_GOAL := default

.PHONY: $(TARGETS)