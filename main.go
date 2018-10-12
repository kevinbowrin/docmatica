package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type PathError struct {
	Path string
	Err  error
}

func main() {
	// Get the current working directory.
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error: Unable to get current working directory, exiting. %v", err)
	}

	// The tool spins up a new goroutine per file.
	// Use a WaitGroup to ensure all processing completes before exiting.
	var wg sync.WaitGroup

	// The linter functions can send errors to this channel.
	lintErrors := make(chan PathError)

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

	// Recursively search the working directory and all subdirectories.
	// Ignore files starting with "."
	err = filepath.Walk(wd, func(path string, info os.FileInfo, err error) error {

		rpath := relPath(path, wd)

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

	anyErrors := make(chan bool)

	// This goroutine prints any errors that into the lintErrors channel.
	go func() {
		tripwire := false
		for pe := range lintErrors {
			fmt.Printf("%v: %v\n", relPath(pe.Path, wd), pe.Err)
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

func check(path string, info os.FileInfo, wg *sync.WaitGroup, lintErrors chan<- PathError) {
	defer wg.Done()
	err := checkFileType(path, info)
	if err != nil {
		lintErrors <- PathError{Path: path, Err: err}
	}
	if filepath.Ext(path) == ".rst" {
		err = checkRstInChapters(path, info)
		if err != nil {
			lintErrors <- PathError{Path: path, Err: err}
		}
		err = checkAnchors(path, info)
		if err != nil {
			lintErrors <- PathError{Path: path, Err: err}
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
		parent(path) != "user-manual" &&
		parent(path) != "getting-started" &&
		parent(path) != "admin-manual" {
		return nil
	}
	if info.Name() == "index.rst" {
		return nil
	}
	if parent(path) == "archivematica-docs" && info.Name() == "contents.rst" {
		return nil
	}

	return errors.New("Not found in chapter directory.")
}

// checkAnchors ensures all pages begin with an anchor and have a back to the top link
// at the bottom of the page, which refers to the page anchor.
func checkAnchors(path string, info os.FileInfo) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	firstLine := true
	foundAnchor := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if firstLine {
			fields := strings.Fields(scanner.Text())
			if len(fields) == 2 &&
				fields[0] == ".." &&
				fields[1][0:1] == "_" &&
				fields[1][len(fields[1])-1:] == ":" {
				foundAnchor = true
			}
		}
		firstLine = false
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if foundAnchor {
		return nil
	}

	return errors.New("Anchor not found on first line.")

}

// Make a relative path from the current working directory and the current path.
func relPath(path, wd string) string {
	return fmt.Sprintf(".%v", strings.TrimPrefix(path, wd))
}

// Get the name of the directory above the end of the path.
func parent(path string) string {
	return filepath.Base(filepath.Dir(path))
}
