TARGETS := $(shell ls scripts|grep -ve "^util-\|entry")
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
.PHONY: $(TARGETS) $(LOCAL_TARGETS) list

list:
	@LC_ALL=C $(MAKE) -pRrq -f $(firstword $(MAKEFILE_LIST)) : 2>/dev/null | awk -v RS= -F: '/(^|\n)# Files(\n|$$)/,/(^|\n)# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' | sort | grep -E -v -e '^[^[:alnum:]]' -e '^$@$$'
# IMPORTANT: The line above must be indented by (at least one)
#            *actual TAB character* - *spaces* do *not* work.
