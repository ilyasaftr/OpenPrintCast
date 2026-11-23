package net

import (
	stdnet "net"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const (
	// cupsDataEnv is the variable ipptool itself reads to relocate its data
	// directory; the tests honor it so they find the same *.test files.
	cupsDataEnv = "CUPS_DATADIR"
	// cupsDataRoot is where cups-ipp-utils installs ipptool's data.
	cupsDataRoot = "/usr/share/cups"
	// cupsToolDirName is the directory below the data root holding the
	// *.test files.
	cupsToolDirName = "ipptool"
	// cupsTestDataEnv names the directory that holds the sample documents
	// fetched by fetchScript, overriding defaultTestDataDir.
	cupsTestDataEnv = "CUPS_TESTDATA_DIR"
	// defaultTestDataDir is where fetchScript puts the documents by default,
	// relative to this package; git ignores it.
	defaultTestDataDir = "testdata/cups-cache"
	// fetchScript downloads the documents the CUPS test files send, which
	// cups-ipp-utils does not install.
	fetchScript = "scripts/fetch-cups-testdata.sh"
	// ipptoolProbeFile is a test file every ipptool installation has.
	ipptoolProbeFile = "get-printer-attributes.test"
	// requestDocument is the document the tests send with ipptool -f.
	requestDocument = "onepage-a4.pdf"
)

// ipptoolEnv is an installed ipptool and the places it reads files from.
//
// Relative file names in a test file (FILE, SKIP-IF-MISSING, INCLUDE) are
// looked up by ipptool in its working directory first and in the directory
// of the test file second; -f names are looked up in the working directory
// first and in the data directory second. Running ipptool in dataDir with
// the *.test files addressed by path therefore searches dataDir first and
// the CUPS data directory (testDir) second.
type ipptoolEnv struct {
	tool    string // path of the ipptool binary
	testDir string // CUPS data directory holding the *.test files
	dataDir string // directory with the fetched sample documents
}

// cupsTestDir returns the directory holding ipptool's own *.test files.
func cupsTestDir() string {
	root := os.Getenv(cupsDataEnv)
	if root == "" {
		root = cupsDataRoot
	}
	return filepath.Join(root, cupsToolDirName)
}

// testDataDir returns the directory of the fetched sample documents.
func testDataDir() string {
	dir := os.Getenv(cupsTestDataEnv)
	if dir == "" {
		dir = defaultTestDataDir
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// requireIPPTool returns the installed ipptool, skipping the test when
// ipptool or its *.test files are missing.
func requireIPPTool(t *testing.T) ipptoolEnv {
	t.Helper()
	tool, err := exec.LookPath("ipptool")
	if err != nil {
		t.Skip("ipptool not installed")
	}
	env := ipptoolEnv{tool: tool, testDir: cupsTestDir(), dataDir: testDataDir()}
	if _, err := os.Stat(env.suite(ipptoolProbeFile)); err != nil {
		t.Skip("CUPS test files not installed")
	}
	return env
}

// suite returns the path of a test file of the CUPS data directory.
func (e ipptoolEnv) suite(name string) string {
	return filepath.Join(e.testDir, name)
}

// command prepares an ipptool run that looks for documents in dataDir first.
func (e ipptoolEnv) command(args ...string) *exec.Cmd {
	cmd := exec.Command(e.tool, args...)
	if info, err := os.Stat(e.dataDir); err == nil && info.IsDir() {
		cmd.Dir = e.dataDir
	}
	return cmd
}

// document returns the path of a sample document, searching dataDir before
// the CUPS data directory. It fails the test, naming fetchScript, when the
// document is in neither: a missing document must not silently weaken a run.
func (e ipptoolEnv) document(t *testing.T, name string) string {
	t.Helper()
	path, ok := e.findDocument(name)
	if !ok {
		t.Fatalf("sample document %s not found in %s or %s: run %s", name, e.dataDir, e.testDir, fetchScript)
	}
	return path
}

// findDocument looks for a sample document in dataDir first and in the CUPS
// data directory second.
func (e ipptoolEnv) findDocument(name string) (path string, found bool) {
	for _, dir := range []string{e.dataDir, e.testDir} {
		path = filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

// startConformanceServer serves the test printer, whose driver is spooler,
// over HTTP for ipptool and returns the printer URI, the server state and
// the ipptool installation. It skips the test when ipptool or its files are
// missing.
func startConformanceServer(t *testing.T, spooler Spooler) (uri string, shared *SharedState, tool ipptoolEnv) {
	t.Helper()
	tool = requireIPPTool(t)
	shared = testShared()
	shared.SetSpooler(spooler)
	server := httptest.NewUnstartedServer(nil)
	host, portString, err := stdnet.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = testMux(shared, host, port)
	server.Start()
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return "ipp://" + u.Host + "/" + core.ResourcePath(testPrinter().ID), shared, tool
}
