ELM_SRC := $(shell find ui/src -name '*.elm')

.PHONY: clean ui-dev test

ui-dev: $(ELM_SRC) ui/elm.json
	@mkdir -p ui/build
	cd ui && elm-esm make src/Main.elm --output=build/app.js

clean:
	rm -rf ui/build

test:
	go test ./...
