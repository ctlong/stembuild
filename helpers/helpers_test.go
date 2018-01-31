package helpers_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/pivotal-cf-experimental/stembuild/helpers"
	"github.com/pivotal-cf-experimental/stembuild/patch"
)

var _ = Describe("Helpers", func() {
	Describe("ManifestString", func() {
		It("Should make an empty manifest properly", func() {
			output, executeErr := StringFromManifest(ManifestTemplate, patch.ApplyPatch{})
			Expect(executeErr).NotTo(HaveOccurred())
			Expect(output).To(Equal(`---
version: ""
vhd_file: ""
patch_file: ""
os_version: ""
output_dir: ""
`))
		})

		It("Should make an empty manifest properly", func() {
			output, executeErr := StringFromManifest(ManifestTemplate, patch.ApplyPatch{
				Version:   "1",
				VHDFile:   "2",
				PatchFile: "3",
				OSVersion: "4",
				OutputDir: "5"})
			Expect(executeErr).NotTo(HaveOccurred())
			Expect(output).To(Equal(`---
version: "1"
vhd_file: "2"
patch_file: "3"
os_version: "4"
output_dir: "5"
`))
		})
	})
})
