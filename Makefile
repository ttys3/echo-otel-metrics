GO_MOD_NAME := $(shell grep -m 1 'module ' go.mod | cut -d' ' -f2)
GO_MOD_DOMAIN := $(shell echo $(GO_MOD_NAME) | awk -F '/' '{print $$1}')
GO_MOD_BASE_NAME := $(shell echo $(GO_MOD_NAME) | awk -F '/' '{print $$NF}')
PROJECT_NAME := $(shell grep 'module ' go.mod | awk '{print $$2}' | sed 's|$(GO_MOD_DOMAIN)/||g')

lint:
	golangci-lint version
	golangci-lint run -v --color always --out-format colored-line-number

fmt:
	@gofumpt -version || go install mvdan.cc/gofumpt@latest
	gofumpt -extra -w -d .
	@gci -v || go install github.com/daixiang0/gci@latest
	gci write -s standard -s default -s 'Prefix($(GO_MOD_DOMAIN))' --skip-generated .

changelog:
	# pacman -S git-cliff
	# brew install git-cliff
	#git-chglog > CHANGELOG.md
	git cliff | tee CHANGELOG.md | bat -l markdown -P