package commandparser_test

import (
	"context"
	"errors"
	"flag"

	"github.com/google/subcommands"

	"github.com/cloudfoundry/stembuild/commandparser/commandparserfakes"

	"github.com/cloudfoundry/stembuild/commandparser"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("package_stemcell", func() {
	// Focus of this test is not to test the Flags.Parse functionality as much
	// as to test that the command line flags values are stored in the expected
	// struct variables. This adds a bit of protection when renaming flag parameters.
	Describe("SetFlags", func() {

		var (
			f      *flag.FlagSet
			PkgCmd *commandparser.PackageCmd

			oSAndVersionGetter *commandparserfakes.FakeOSAndVersionGetter
			packagerFactory    *commandparserfakes.FakePackagerFactory
			packager           *commandparserfakes.FakePackager
			packagerMessenger  *commandparserfakes.FakePackagerMessenger
		)

		BeforeEach(func() {
			f = flag.NewFlagSet("test", flag.ContinueOnError)

			oSAndVersionGetter = new(commandparserfakes.FakeOSAndVersionGetter)
			packagerFactory = new(commandparserfakes.FakePackagerFactory)
			packager = new(commandparserfakes.FakePackager)
			packagerMessenger = new(commandparserfakes.FakePackagerMessenger)

			packagerFactory.PackagerReturns(packager, nil)

			PkgCmd = commandparser.NewPackageCommand(oSAndVersionGetter, packagerFactory, packagerMessenger)
			PkgCmd.SetFlags(f)
			PkgCmd.GlobalFlags = &commandparser.GlobalFlags{false, false, false}
		})

		var defaultArgs = []string{}

		Describe("Execute", func() {
			BeforeEach(func() {
				oSAndVersionGetter.GetVersionReturns("2019.2")
				oSAndVersionGetter.GetOsReturns("2019")
			})

			It("packager is instantiated with expected vmdk source config", func() {
				vmdk_args := []string{"-vmdk", "some_vmdk_file"}

				err := f.Parse(vmdk_args)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitSuccess))

				Expect(packagerFactory.PackagerCallCount()).To(Equal(1))
				actualSourceConfig, _, _, _ := packagerFactory.PackagerArgsForCall(0)
				Expect(actualSourceConfig.Vmdk).To(Equal("some_vmdk_file"))
			})

			It("packager is instantiated with expected vcenter source config", func() {
				vcenter_args := []string{
					"-vcenter-url", "https://vcenter.test",
					"-vcenter-username", "test-user",
					"-vcenter-password", "verysecure",
					"-vcenter-ca-certs", "/path/to/cert/file",
					"-vm-inventory-path", "/path/to/vm",
				}

				err := f.Parse(vcenter_args)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitSuccess))

				Expect(packagerFactory.PackagerCallCount()).To(Equal(1))
				actualSourceConfig, _, _, _ := packagerFactory.PackagerArgsForCall(0)
				Expect(actualSourceConfig.URL).To(Equal("https://vcenter.test"))
				Expect(actualSourceConfig.Username).To(Equal("test-user"))
				Expect(actualSourceConfig.Password).To(Equal("verysecure"))
				Expect(actualSourceConfig.VmInventoryPath).To(Equal("/path/to/vm"))
				Expect(actualSourceConfig.CaCertFile).To(Equal("/path/to/cert/file"))
			})

			It("packager is instantiated with expected output config directory when using long form -outputdir", func() {
				longformOutputDirArgs := []string{"-outputDir", "some_output_dir"}

				err := f.Parse(longformOutputDirArgs)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitSuccess))

				Expect(packagerFactory.PackagerCallCount()).To(Equal(1))
				_, actualOutputConfig, _, _ := packagerFactory.PackagerArgsForCall(0)
				Expect(actualOutputConfig.OutputDir).To(Equal("some_output_dir"))
			})

			It("packager is instantiated with expected output config when using short form -o", func() {
				shortformOutputDirArgs := []string{"-o", "some_output_dir"}

				err := f.Parse(shortformOutputDirArgs)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitSuccess))

				Expect(packagerFactory.PackagerCallCount()).To(Equal(1))
				_, actualOutputConfig, _, _ := packagerFactory.PackagerArgsForCall(0)
				Expect(actualOutputConfig.OutputDir).To(Equal("some_output_dir"))
				Expect(actualOutputConfig.StemcellVersion).To(Equal("2019.2"))
				Expect(actualOutputConfig.Os).To(Equal("2019"))
			})

			It("creates packager with correct stemcell patch version number when argument provided", func() {
				oSAndVersionGetter.GetVersionWithPatchNumberReturns("1803.27.36")

				args := append(defaultArgs, "-patch-version", "36")

				err := f.Parse(args)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitSuccess))

				Expect(packagerFactory.PackagerCallCount()).To(Equal(1))
				_, actualOutputConfig, _, _ := packagerFactory.PackagerArgsForCall(0)
				Expect(actualOutputConfig.StemcellVersion).To(Equal("1803.27.36"))

				Expect(oSAndVersionGetter.GetVersionWithPatchNumberCallCount()).To(Equal(1))
				actualPatchVersion := oSAndVersionGetter.GetVersionWithPatchNumberArgsForCall(0)
				Expect(actualPatchVersion).To(Equal("36"))
			})

			It("package is not called if the OS is invalid", func() {
				oSAndVersionGetter.GetOsReturns("2017")

				err := f.Parse(defaultArgs)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitFailure))

				Expect(packager.PackageCallCount()).To(Equal(0))

				Expect(packagerMessenger.InvalidOutputConfigCallCount()).To(Equal(1))
				receivedError := packagerMessenger.InvalidOutputConfigArgsForCall(0)
				Expect(receivedError.Error()).To(MatchRegexp("2017"))
			})

			It("package is not called if the packager factory returns an error", func() {
				packagerFactory.PackagerReturns(nil, errors.New("Couldn't make a packager!"))

				err := f.Parse(defaultArgs)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitFailure))

				Expect(packagerFactory.PackagerCallCount()).To(Equal(1))
				Expect(packager.PackageCallCount()).To(Equal(0))

				Expect(packagerMessenger.CannotCreatePackagerCallCount()).To(Equal(1))
				receivedError := packagerMessenger.CannotCreatePackagerArgsForCall(0)
				Expect(receivedError).To(MatchError("Couldn't make a packager!"))
			})

			It("package is not called if there is no free space", func() {
				packager.ValidateFreeSpaceForPackageReturns(errors.New("No space!"))

				err := f.Parse(defaultArgs)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitFailure))

				Expect(packager.ValidateFreeSpaceForPackageCallCount()).To(Equal(1))
				Expect(packager.PackageCallCount()).To(Equal(0))

				Expect(packagerMessenger.DoesNotHaveEnoughSpaceCallCount()).To(Equal(1))
				receivedError := packagerMessenger.DoesNotHaveEnoughSpaceArgsForCall(0)
				Expect(receivedError).To(MatchError("No space!"))
			})

			It("package is not called if source parameters are not valid", func() {
				packager.ValidateSourceParametersReturns(errors.New("invalid source parameters"))

				err := f.Parse(defaultArgs)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitFailure))

				Expect(packager.ValidateSourceParametersCallCount()).To(Equal(1))
				Expect(packager.PackageCallCount()).To(Equal(0))

				Expect(packagerMessenger.SourceParametersAreInvalidCallCount()).To(Equal(1))
				receivedError := packagerMessenger.SourceParametersAreInvalidArgsForCall(0)
				Expect(receivedError).To(MatchError("invalid source parameters"))
			})

			It("exits with failure if package returns an error", func() {
				packager.PackageReturns(errors.New("Didn't make it"))

				err := f.Parse(defaultArgs)
				Expect(err).ToNot(HaveOccurred())

				exitStatus := PkgCmd.Execute(context.Background(), f)
				Expect(exitStatus).To(Equal(subcommands.ExitFailure))

				Expect(packager.PackageCallCount()).To(Equal(1))

				Expect(packagerMessenger.PackageFailedCallCount()).To(Equal(1))
				receivedError := packagerMessenger.PackageFailedArgsForCall(0)
				Expect(receivedError).To(MatchError("Didn't make it"))
			})
		})
	})
})
