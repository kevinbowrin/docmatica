package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

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
	lintErrors := make(chan error)

	// Recursively search the working directory and all subdirectories.
	// Ignore files starting with "."
	err = filepath.Walk(wd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		wg.Add(1)
		go check(path, info, &wg, lintErrors)
		return nil
	})
	if err != nil {
		log.Printf("Warning: File access error during recursive search. %v", err)
	}

	anyErrors := make(chan bool)

	go func() {
		tripwire := false
		for err := range lintErrors {
			fmt.Println(err)
			tripwire = true
		}
		if tripwire {
			anyErrors <- true
		} else {
			anyErrors <- false
		}
	}()

	wg.Wait()
	close(lintErrors)
	wasThereErrors := <-anyErrors
	if wasThereErrors {
		os.Exit(1)
	}
}

func check(path string, info os.FileInfo, wg *sync.WaitGroup, lintErrors chan<- error) {
	defer wg.Done()
	err := checkFileType(path, info)
	if err != nil {
		lintErrors <- err
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
	if filepath.Base(filepath.Dir(path)) == "images" {
		if filepath.Ext(path) == ".png" || filepath.Ext(path) == ".svg" {
			return nil
		}
	}

	return fmt.Errorf("File at path %v does not have a .rst file extension. "+
		"It also does not have a .png or .svg file extension and reside in an 'images' directory.", path)
}
