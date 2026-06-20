.PHONY: install-hooks

install-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed from .githooks/"
