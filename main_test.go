package main

import (
	"testing"
)

func TestRelPath(t *testing.T) {

	testTable := []struct {
		path     string
		wd       string
		expected string
	}{
		{"/a/b/c", "/a/b", "./c"},
		{"/a/b/c/test.txt", "/a/b", "./c/test.txt"},
	}

	for _, r := range testTable {
		result := relPath(r.path, r.wd)
		if result != r.expected {
			t.Errorf("relPath(%v, %v) -> %v, not %v", r.path, r.wd, result, r.expected)
		}
	}

}

func TestParent(t *testing.T) {

	testTable := []struct {
		path     string
		expected string
	}{
		{"/a/b/c", "b"},
		{"./a/test.txt", "a"},
		{".", "."},
	}

	for _, r := range testTable {
		result := parent(r.path)
		if result != r.expected {
			t.Errorf("parent(%v) -> %v, not %v", r.path, result, r.expected)
		}
	}

}
