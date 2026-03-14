UV ?= uv
.PHONY: install test run lint build
install:
	$(UV) sync --all-extras
test:
	$(UV) run pytest -q
run:
	$(UV) run python -m codecontext -help
lint:
	$(UV) run python -m compileall -q src
build:
	$(UV) build