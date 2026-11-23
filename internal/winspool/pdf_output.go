package winspool

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// allocatePDFOutput reserves a directory instead of an existing file, since
// the Windows PDF driver creates the file itself.
func allocatePDFOutput(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("PDF output directory is unavailable")
	}
	dir := filepath.Join(root, "OpenPrintCast", "PDFs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return allocatePDFOutputIn(dir)
}

// allocatePDFJobOutput picks the staging path for a PDF job: inside folder
// when one is requested, otherwise under the user's local application data.
func allocatePDFJobOutput(folder string) (string, error) {
	if folder != "" {
		return wrapPDFAllocation(allocatePDFOutputIn(folder))
	}
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		var err error
		root, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("locate PDF output directory: %w", err)
		}
	}
	return wrapPDFAllocation(allocatePDFOutput(root))
}

func wrapPDFAllocation(output string, err error) (string, error) {
	if err != nil {
		return "", fmt.Errorf("allocate PDF output: %w", err)
	}
	return output, nil
}

func allocatePDFOutputIn(dir string) (string, error) {
	jobDir, err := os.MkdirTemp(dir, "job-")
	if err != nil {
		return "", err
	}
	return filepath.Join(jobDir, "AirPrint.pdf"), nil
}

// Folder outputs are named after the job with a random suffix, like
// os.CreateTemp, and never replace an existing file.
const publishAttempts = 100

// publishFolderPDF gives the finished PDF at staged its final, unused name in
// dir. Where the file system allows it the file is hard-linked, so it appears
// complete in one step; otherwise it is copied into an exclusively created
// destination file so existing files stay safe.
func publishFolderPDF(staged, dir, jobName string) (string, error) {
	path, err := linkFolderPDF(staged, dir, jobName)
	if err != nil {
		path, err = copyFolderPDF(staged, dir, jobName)
	}
	if err != nil {
		return "", err
	}
	removeStagedPDF(staged)
	return path, nil
}

// uniquePDFName returns a candidate destination name for jobName in dir.
func uniquePDFName(dir, jobName string) string {
	return filepath.Join(dir, pdfNameStem(jobName)+"-"+strconv.FormatUint(uint64(rand.Uint32()), 10)+".pdf")
}

func linkFolderPDF(staged, dir, jobName string) (string, error) {
	var err error
	for range publishAttempts {
		path := uniquePDFName(dir, jobName)
		if err = os.Link(staged, path); err == nil {
			return path, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	return "", err
}

func copyFolderPDF(staged, dir, jobName string) (string, error) {
	source, err := os.Open(staged)
	if err != nil {
		return "", err
	}
	defer source.Close()

	output, err := os.CreateTemp(dir, pdfNameStem(jobName)+"-*.pdf")
	if err != nil {
		return "", err
	}
	path := output.Name()
	_, copyErr := io.Copy(output, source)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		if err := os.Remove(path); err != nil {
			componentLogger(componentPDF).Warn("remove incomplete pdf failed", "path", path, "err", err)
		}
		if copyErr != nil {
			return "", copyErr
		}
		return "", closeErr
	}
	return path, nil
}

// removeStagedPDF removes only the temporary file and directory created for
// this job.
func removeStagedPDF(staged string) {
	if err := os.Remove(staged); err != nil {
		componentLogger(componentPDF).Warn("remove staged pdf failed", "path", staged, "err", err)
	} else if err := os.Remove(filepath.Dir(staged)); err != nil {
		componentLogger(componentPDF).Warn("remove pdf staging directory failed", "path", filepath.Dir(staged), "err", err)
	}
}

func pdfNameStem(jobName string) string {
	// IPP names are untrusted and may include either kind of path separator.
	if i := strings.LastIndexAny(jobName, `/\`); i >= 0 {
		jobName = jobName[i+1:]
	}
	if i := strings.LastIndex(jobName, "."); i > 0 {
		jobName = jobName[:i]
	}
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, jobName)
	name = strings.Trim(name, " .")
	runes := []rune(name)
	if len(runes) > 80 {
		name = strings.Trim(string(runes[:80]), " .")
	}
	if name == "" {
		return "AirPrint"
	}
	device := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if device == "CON" || device == "PRN" || device == "AUX" || device == "NUL" ||
		(len(device) == 4 && (strings.HasPrefix(device, "COM") || strings.HasPrefix(device, "LPT")) && device[3] >= '1' && device[3] <= '9') ||
		device == "COM\u00b9" || device == "COM\u00b2" || device == "COM\u00b3" ||
		device == "LPT\u00b9" || device == "LPT\u00b2" || device == "LPT\u00b3" {
		name = "_" + name
	}
	return name
}

// completePDFOutput reports whether path holds a PDF header and a complete
// trailer, which a folder job requires before it publishes the file.
func completePDFOutput(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() < 10 {
		return false
	}
	header := make([]byte, 5)
	if _, err := io.ReadFull(f, header); err != nil || !bytes.Equal(header, []byte("%PDF-")) {
		return false
	}
	tailSize := min(info.Size(), 2048)
	tail := make([]byte, tailSize)
	if _, err := f.ReadAt(tail, info.Size()-tailSize); err != nil {
		return false
	}
	return bytes.HasSuffix(bytes.TrimSpace(tail), []byte("%%EOF"))
}
