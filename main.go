package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type pathError struct {
	path string
	err  error
}

var (
	pathFlag = flag.String("path", "", "The path to the directory you want to run the tool on. "+
		"If not provided, the current working directory will be used.")
	// A version flag, which should be overwritten when building using ldflags.
	version = "devel"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Docmatica\nVersion %v\n\n", version)
		fmt.Fprintln(os.Stderr, "A linter for archivematica-docs.")
		fmt.Fprintln(os.Stderr, "This tool works best when run at the root of the archivematica-docs repository.")
		fmt.Fprintln(os.Stderr, "The following checks will be performed:")
		fmt.Fprintln(os.Stderr, "- All files found have extension .rst or .svg or .png in an images directory.")
		fmt.Fprintln(os.Stderr, "- All .rst files are nested within chapter directories, except:")
		fmt.Fprintln(os.Stderr, "    * index.rst files, which can be in the root of manuals or the root of the repository.")
		fmt.Fprintln(os.Stderr, "    * contents.rst files, which can be in the root of the repository.")
		fmt.Fprintln(os.Stderr, "- All .rst files have 'Back to Top' anchors.")
		fmt.Fprintln(os.Stderr, "\nCommand line arguments:\n")
		flag.PrintDefaults()
	}
}

func main() {

	// Process the flags.
	flag.Parse()

	root := *pathFlag

	if root == "" {
		// Get the current working directory.
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Error: Unable to get current working directory, exiting. %v", err)
		}
		root = wd
	}

	// The tool spins up a new goroutine per file.
	// Use a WaitGroup to ensure all processing completes before exiting.
	var wg sync.WaitGroup

	// The linter functions can send errors to this channel.
	lintErrors := make(chan pathError)

	// These are the names of files we can ignore
	// when we're in the "archivematica-docs" directory.
	ignore := []string{
		"requirements.txt",
		"README.md",
		"Makefile",
		"LICENCE",
		"issue_template.md",
		"conf.py",
	}

	// Recursively search the root directory and all subdirectories.
	// Ignore files starting with "."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

		rpath := relPath(path, root)

		// If an error occurred accessing this path, print it but don't stop processing.
		if err != nil {
			log.Printf("Error with path %v: %v", rpath, err)
			return nil
		}

		// If the name starts with ".", skip it.
		if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// If the name starts with "_", skip it.
		if strings.HasPrefix(info.Name(), "_") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// If we're in the "archivematica-docs" directory, it's a special case.
		// Ignore some files and directories.
		if parent(path) == "archivematica-docs" {
			if info.Name() == "locale" && info.IsDir() {
				return filepath.SkipDir
			}
			if info.Name() == "_static" && info.IsDir() {
				return filepath.SkipDir
			}
			for _, i := range ignore {
				if info.Name() == i {
					return nil
				}
			}
		}

		wg.Add(1)
		go check(path, info, &wg, lintErrors)
		return nil
	})
	if err != nil {
		log.Printf("Warning: File access error during recursive search. %v", err)
	}

	anyErrors := make(chan bool, 1)

	// This goroutine prints any errors that into the lintErrors channel.
	go func() {
		tripwire := false
		for pe := range lintErrors {
			fmt.Printf("%v: %v\n", relPath(pe.path, root), pe.err)
			tripwire = true
		}

		// If even one error happened, pass false back to the parent thread.
		if tripwire {
			anyErrors <- true
		} else {
			anyErrors <- false
		}
	}()

	// Wait for the processing goroutines to finish.
	wg.Wait()
	close(lintErrors)

	// If any errors occurred, exit with a 1 error code.
	wasThereErrors := <-anyErrors
	if wasThereErrors {
		os.Exit(1)
	}
}

func check(path string, info os.FileInfo, wg *sync.WaitGroup, lintErrors chan<- pathError) {
	defer wg.Done()
	err := checkFileType(path, info)
	if err != nil {
		lintErrors <- pathError{path: path, err: err}
	}
	if filepath.Ext(path) == ".rst" {
		err = checkRstInChapters(path, info)
		if err != nil {
			lintErrors <- pathError{path: path, err: err}
		}
		err = checkFileContent(path, lintErrors)
		if err != nil {
			lintErrors <- pathError{path: path, err: err}
		}
	}
}

// checkFileType ensures all files found have extension .rst or
// were .svg or .png in an images directory.
func checkFileType(path string, info os.FileInfo) error {
	if info.IsDir() {
		return nil
	}
	if filepath.Ext(path) == ".rst" {
		return nil
	}
	if parent(path) == "images" {
		if filepath.Ext(path) == ".png" || filepath.Ext(path) == ".svg" {
			return nil
		}
	}

	return errors.New("Does not have a .rst file extension or a .png or .svg extension while nested in an 'images' directory.")
}

// checkRstInChapters ensures that all reST files are nested within chapter directories
// with the exception of the following:
// contents.rst - the top-level toctree for the documentation
// index.rst - the main index for the documentation, which acts as the homepage
func checkRstInChapters(path string, info os.FileInfo) error {
	if parent(path) != "archivematica-docs" &&
		parent(path) != "admin-manual" &&
		parent(path) != "getting-started" &&
		parent(path) != "user-manual" &&
		parent(path) != "images" {
		return nil
	}
	if parent(path) == "archivematica-docs" &&
		(info.Name() == "index.rst" || info.Name() == "contents.rst") {
		return nil
	}
	if (parent(path) == "admin-manual" ||
		parent(path) == "getting-started" ||
		parent(path) == "user-manual") &&
		info.Name() == "index.rst" {
		return nil
	}

	return errors.New("Not found in chapter directory.")
}

func checkFileContent(path string, lintErrors chan<- pathError) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	anchorLines := make(chan string)
	anchorError := make(chan error, 1)
	go checkAnchors(anchorLines, anchorError)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		anchorLines <- scanner.Text()
	}
	close(anchorLines)
	err, errValid := <-anchorError
	if errValid {
		lintErrors <- pathError{path: path, err: err}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// checkAnchors ensures all pages begin with an anchor and have a back to the top link
// at the bottom of the page, which refers to the page anchor.
func checkAnchors(lines <-chan string, errC chan<- error) {
	defer close(errC)
	firstLine := true
	foundAnchor := false
	matchingAnchor := false
	anchorText := ""
	for line := range lines {
		fields := strings.Fields(line)
		if firstLine {
			if len(fields) == 2 &&
				fields[0] == ".." &&
				fields[1][0:1] == "_" &&
				fields[1][len(fields[1])-1:] == ":" {
				anchorText = fields[1][1 : len(fields[1])-1]
				foundAnchor = true
			}
			firstLine = false
		}
		if foundAnchor {
			if !matchingAnchor {
				if line == fmt.Sprintf(":ref:`Back to the top <%v>`", anchorText) {
					matchingAnchor = true
				}
			}
		}
	}
	if !foundAnchor {
		errC <- errors.New("Anchor not found at top of page.")
	} else if !matchingAnchor {
		errC <- errors.New("'Back to top' link to anchor not found.")
	}
}

// Make a relative path from the current root and the current path.
func relPath(path, root string) string {
	return fmt.Sprintf(".%v", strings.TrimPrefix(path, root))
}

// Get the name of the directory above the end of the path.
func parent(path string) string {
	return filepath.Base(filepath.Dir(path))
}
