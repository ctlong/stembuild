package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pivotal-cf-experimental/stembuild/ovftool"
)

// Run this test first - no point continuing with ovftool!
func TestOVFTool(t *testing.T) {
	const format = "locating ovftool executable: %s\n\n" +
		"The OVF Tool is required to run these tests.\n" +
		"It may be download from VMware for free from:\n" +
		"  https://www.vmware.com/support/developer/ovf/\n\n"
	if _, err := ovftool.Ovftool(); err != nil {
		t.Logf(format, err)
		panic("stopping tests now - missing ovftool!")
	}
}

var (
	InitRan = false
)

func runInit() {
	if !InitRan {
		Init()
		InitRan = true
	}
}

func readdirnames(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func TestOsVersion(t *testing.T) {
	const archive = "testdata/patch-test.tar.gz"

	// Reset Version and OSVersion on exit
	oldVersion := applyPatch.Version
	oldOSVersion := applyPatch.OSVersion
	defer func() {
		applyPatch.Version = oldVersion
		applyPatch.OSVersion = oldOSVersion
	}()
	applyPatch.Version = "9000"
	applyPatch.OSVersion = "2016"

	var err error
	outputDir, err := ioutil.TempDir("", "test-")
	if err != nil {
		t.Fatal(err)
	}
	applyPatch.OutputDir = outputDir
	defer os.RemoveAll(applyPatch.OutputDir)

	dirname := extractGzipArchive(t, archive)
	defer os.RemoveAll(dirname)
	vmdkPath := filepath.Join(dirname, "expected.vmdk")

	conf := Config{stop: make(chan struct{})}

	realMain(&conf, vmdkPath, "", "")

	// assertions
	stemcellFilename := filepath.Base(conf.Stemcell)
	stemcellDirname := extractGzipArchive(t, filepath.Join(applyPatch.OutputDir, stemcellFilename))
	manifestFilepath := filepath.Join(stemcellDirname, "stemcell.MF")

	manifest, err := readFile(manifestFilepath)
	if err != nil {
		t.Fatal(err)
	}

	expectedOs := fmt.Sprintf("operating_system: windows%s", applyPatch.OSVersion)
	if !strings.Contains(manifest, expectedOs) {
		t.Errorf("TestOSVerson: stemcell.MF expected os: %s\n%s\n",
			expectedOs, manifest)
	}

	expectedName := fmt.Sprintf("name: bosh-vsphere-esxi-windows%s-go_agent", applyPatch.OSVersion)
	if !strings.Contains(manifest, expectedName) {
		t.Errorf("TestOSVerson: stemcell.MF expected stemcell filename: %s\n%s\n",
			expectedName, manifest)
	}

	if !strings.Contains(stemcellFilename, applyPatch.OSVersion) {
		t.Errorf("TestOSVerson: expected filename: %s got: %s",
			applyPatch.OSVersion, stemcellFilename)
	}
}

func TestExtractOVA_Valid(t *testing.T) {
	const Count = 9
	const NameFmt = "file-%d"

	tmpdir, err := ioutil.TempDir("", "test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	if err := ExtractOVA("testdata/tar/valid.tar", tmpdir); err != nil {
		t.Fatal(err)
	}

	var expFileNames []string
	for i := 0; i <= Count; i++ {
		expFileNames = append(expFileNames, fmt.Sprintf("file-%d", i))
	}
	sort.Strings(expFileNames)

	names, err := readdirnames(tmpdir)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(expFileNames, names) {
		t.Errorf("ExtractOVA: filenames want: %v got: %v", expFileNames, names)
	}

	// the content of each file is it's index
	// and a newline so 'file-2' contains "2\n"
	validFile := func(name string) error {
		path := filepath.Join(tmpdir, name)
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		var i int
		if _, err := fmt.Sscanf(name, NameFmt, &i); err != nil {
			return err
		}
		exp := fmt.Sprintf("%d\n", i)
		if s := string(b); s != exp {
			t.Errorf("ExtractOVA: file (%s) want: %s got: %s", name, exp, s)
		}
		return nil
	}

	for _, name := range names {
		if err := validFile(name); err != nil {
			t.Error(err)
		}
	}
}

func TestExtractOVA_Invalid(t *testing.T) {
	var tests = []struct {
		archive string
		reason  string
	}{
		{
			"has-sub-dir.tar",
			"subdirectories are not supported",
		},
		{
			"too-many-files.tar",
			"too many files read from archive (this is capped at 100)",
		},
		{
			"symlinks.tar",
			"symlinks are not supported",
		},
	}

	for _, x := range tests {
		tmpdir, err := ioutil.TempDir("", "test-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpdir)

		filename := filepath.Join("testdata", "tar", x.archive)
		if err := ExtractOVA(filename, tmpdir); err == nil {
			t.Errorf("ExtractOVA (%s): expected error because:", x.archive, x.reason)
		}
	}
}

func TestApplyPatch(t *testing.T) {
	const archive = "testdata/patch-test.tar.gz"

	dirname := extractGzipArchive(t, archive)
	defer os.RemoveAll(dirname)

	vhd := filepath.Join(dirname, "original.vhd")
	patch := filepath.Join(dirname, "delta.patch")
	expVMDK := filepath.Join(dirname, "expected.vmdk")

	// output file
	newVMDK := filepath.Join(dirname, "image.vmdk")

	conf := Config{stop: make(chan struct{})}

	// Normal operation
	{
		if err := conf.ApplyPatch(vhd, patch, newVMDK); err != nil {
			t.Fatal(err)
		}
		expSrc, err := ioutil.ReadFile(expVMDK)
		if err != nil {
			t.Fatalf("ApplyPatch: reading expected vmdk file (%s): %s", expVMDK, err)
		}
		vmdkSrc, err := ioutil.ReadFile(newVMDK)
		if err != nil {
			t.Fatalf("ApplyPatch: reading vmdk file (%s): %s", newVMDK, err)
		}
		if !bytes.Equal(expSrc, vmdkSrc) {
			t.Fatalf("ApplyPatch: patched vmdk (%s) does not match expected vmdk (%s)",
				newVMDK, expVMDK)
		}
	}

	// Error when VMDK file already exists
	{
		if err := conf.ApplyPatch(vhd, patch, newVMDK); err == nil {
			t.Error("ApplyPatch: expected an error when the VMDK already exists got")
		}
	}
}

func TestCreateImage(t *testing.T) {
	const archive = "testdata/patch-test.tar.gz"

	dirname := extractGzipArchive(t, archive)
	defer os.RemoveAll(dirname)

	vmdkPath := filepath.Join(dirname, "expected.vmdk")

	conf := Config{stop: make(chan struct{})}

	if err := conf.CreateImage(vmdkPath); err != nil {
		t.Errorf("CreateImage: %s", err)
	}

	// the image will be saved to the Config's temp directory
	tmpdir, err := conf.TempDir()
	if err != nil {
		t.Error(err)
	}
	expImagePath := filepath.Join(tmpdir, "image")

	if conf.Image != expImagePath {
		t.Errorf("CreateImage: expected ImagePath to be: %s got: %s",
			expImagePath, conf.Image)
	}

	// Make sure the sha1 sum is correct
	h := sha1.New()
	f, err := os.Open(conf.Image)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", h.Sum(nil))

	if conf.Sha1sum != sum {
		t.Errorf("CreateImage: expected sha1: %s got: %s", sum, conf.Sha1sum)
	}

	// extract image
	{
		// expect the image ova to contain only the following file names
		expectedNames := []string{
			"image.ovf",
			"image.mf",
			"image-disk1.vmdk",
		}

		imageDir := extractGzipArchive(t, conf.Image)
		list, err := ioutil.ReadDir(imageDir)
		if err != nil {
			t.Fatal(err)
		}

		var names []string
		infos := make(map[string]os.FileInfo)
		for _, fi := range list {
			names = append(names, fi.Name())
			infos[fi.Name()] = fi
		}

		if len(names) != 3 {
			t.Errorf("CreateImage: expected image (%s) to contain 3 files, found: %d - %s",
				imageDir, len(names), names)
		}
		for _, name := range expectedNames {
			if _, ok := infos[name]; !ok {
				t.Errorf("CreateImage: image (%s) is missing file: %s", names, name)
			}
		}

		// the vmx template should generate an ovf file that
		// does not contain an ethernet section.
		//
		ovf := filepath.Join(imageDir, "image.ovf")
		s, err := readFile(ovf)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(s), "ethernet") {
			t.Errorf("CreateImage: ovf contains 'ethernet' block:\n%s\n", s)
		}
	}
}

// this checks that CreateImage can take the relative path of a VMDK
func TestCreateImagePathResolution(t *testing.T) {
	const archive = "testdata/patch-test.tar.gz"

	dirname := extractGzipArchive(t, archive)
	defer os.RemoveAll(dirname)

	// get current working dir
	cwd, err := os.Getwd()
	if err != nil {
		t.Errorf("could not get working dir")
	}

	if err := os.Chdir(dirname); err != nil {
		t.Errorf("Could not change to test tmp dir: %s", dirname)
	}

	conf := Config{stop: make(chan struct{})}

	if err := conf.CreateImage("expected.vmdk"); err != nil {
		t.Errorf("CreateImage couldn't expand absolute path of VMDK file: %s", err)
	}

	// change back to current working dir
	if err := os.Chdir(cwd); err != nil {
		t.Errorf("Could not change back to working dir: %s", cwd)
	}
}

func TestThatTheManifestIsGeneratedCorrectly(t *testing.T) {
	result := CreateManifest("1", "version", "sha1sum")
	expectedManifest := `---
name: bosh-vsphere-esxi-windows1-go_agent
version: 'version'
sha1: sha1sum
operating_system: windows1
cloud_properties:
  infrastructure: vsphere
  hypervisor: esxi
stemcell_formats:
- vsphere-ovf
- vsphere-ova
`
	if result != expectedManifest {
		t.Errorf("result:\n%s\ndoes not match expected\n%s\n", result, expectedManifest)
	}
}

func TestValidApplyPatchManifestFile(t *testing.T) {
	testCommand := fmt.Sprintf(
		"stembuild apply-patch %s",
		"testdata/valid-apply-patch.yml",
	)
	testArgs := strings.Split(testCommand, " ")
	os.Args = testArgs
	runInit()
	ParseFlags()

	errs := ValidateFlags()

	if len(errs) != 0 {
		t.Errorf("expected no errors, but got errors: %s", errs)
	}
}

func TestInvalidApplyPatchManifestFile(t *testing.T) {
	testCommand := fmt.Sprintf(
		"stembuild apply-patch %s",
		"testdata/invalid-apply-patch.yml",
	)
	testArgs := strings.Split(testCommand, " ")
	os.Args = testArgs
	runInit()
	ParseFlags()

	errs := ValidateFlags()

	if len(errs) != 1 {
		t.Error("expected single error; but got no, or more than one, error(s)")
	}
}

func readFile(name string) (string, error) {
	b, err := ioutil.ReadFile(name)
	return string(b), err
}

// extractGzipArchive, extracts the tgz archive name to a temp directory
// returning the filepath of the temp directory.
func extractGzipArchive(t *testing.T, name string) string {
	t.Logf("extractGzipArchive: extracting tgz: %s", name)

	tmpdir, err := ioutil.TempDir("", "test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("extractGzipArchive: using temp directory: %s", tmpdir)

	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExtractArchive(w, tmpdir); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return tmpdir
}
