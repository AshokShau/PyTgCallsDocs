package docs

import (
	"testing"
)

func TestSearch(t *testing.T) {
	d := Documentation{
		"test1": &DocEntry{
			Title:       "Add Incoming Video",
			Description: "This method allows to notify the library",
			Lib:         "NTgCalls",
		},
		"test2": &DocEntry{
			Title:       "AudioParameters",
			Description: "Stream's Audio Configuration",
			Lib:         "PyTgCalls",
		},
	}

	results := d.Search("incoming", 10)
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Add Incoming Video" {
		t.Errorf("Expected 'Add Incoming Video', got '%s'", results[0].Title)
	}

	results = d.Search("Audio", 10)
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}
