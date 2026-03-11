package docs

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

type DocItem struct {
	Name         string  `json:"name"`
	Type         *string `json:"type"`
	Description  string  `json:"description"`
	SourceConfig *string `json:"source_config"`
	Value        *string `json:"value"`
}

type Example struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

type Section struct {
	Title string    `json:"title"`
	Items []DocItem `json:"items"`
}

type Details struct {
	Sections   []Section `json:"sections,omitempty"`
	Signature  *string   `json:"signature"`
	Members    []DocItem `json:"members,omitempty"`
	Properties []DocItem `json:"properties,omitempty"`
	Parameters []DocItem `json:"parameters,omitempty"`
}

type DocEntry struct {
	Path        string   `json:"-"`
	Title       string   `json:"title"`
	Lib         string   `json:"lib"`
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Example     *Example `json:"example,omitempty"`
	Details     Details  `json:"details"`
	DocURL      string   `json:"doc_url"`
}

type Documentation map[string]*DocEntry

func Load(path string) (Documentation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var docs Documentation
	err = json.Unmarshal(data, &docs)
	if err != nil {
		return nil, err
	}

	for p, entry := range docs {
		entry.Path = p
	}

	return docs, nil
}

type SearchResult struct {
	Entry *DocEntry
	Score int
}

func (d Documentation) Search(query string, limit int) []*DocEntry {
	if query == "" {
		return nil
	}

	query = strings.ToLower(query)
	var results []SearchResult

	for _, entry := range d {
		score := 0
		title := strings.ToLower(entry.Title)
		
		if title == query {
			score += 100
		} else if strings.HasPrefix(title, query) {
			score += 50
		} else if strings.Contains(title, query) {
			score += 30
		}

		desc := strings.ToLower(entry.Description)
		if strings.Contains(desc, query) {
			score += 10
		}

		if score > 0 {
			results = append(results, SearchResult{Entry: entry, Score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Entry.Title < results[j].Entry.Title
		}
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	final := make([]*DocEntry, len(results))
	for i, r := range results {
		final[i] = r.Entry
	}

	return final
}
