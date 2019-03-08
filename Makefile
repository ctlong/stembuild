LD_FLAGS = "-w -s"
GOSRC = $(shell find . -name "*.go" ! -name "*test.go" ! -name "*fake*" ! -path "./integration/*")
COMMAND = out/stembuild
AUTOMATION_PATH = integration/construct/assets/StemcellAutomation.zip
AUTOMATION_PREFIX = $(shell dirname "${AUTOMATION_PATH}")

all : test build

build : out/stembuild

clean :
	rm -r version/version.go
	rm -r $(wildcard out/*)

format :
	go fmt ./...

integration : build
	ginkgo -r -v -randomizeAllSpecs integration

out/stembuild : $(GOSRC)
	go generate
	go get -u github.com/jteeuwen/go-bindata/...
	go-bindata -o stemcell_automation.go -prefix $(AUTOMATION_PREFIX) $(AUTOMATION_PATH)
	go build -o $(COMMAND) -ldflags $(LD_FLAGS) .

test : units

units : format build
	@ginkgo version
	ginkgo -r -v -randomizeAllSpecs -randomizeSuites -skipPackage integration,iaas_cli
	@echo "\nSWEET SUITE SUCCESS"

contract :
	ginkgo -r -v -randomizeAllSpecs -randomizeSuites iaas_cli

.PHONY : all build clean format
.PHONY : test units units-full integration integration-tests-full
