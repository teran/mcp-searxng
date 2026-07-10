package domain

import (
	"testing"
)

func TestSearchResult_Populate(t *testing.T) {
	t.Parallel()

	pubDate := "2024-01-15T10:00:00Z"
	imgSrc := "https://example.com/image.jpg"
	source := "example.com"

	r := SearchResult{
		Title:         "Test Result",
		URL:           "https://example.com/page",
		Content:       "This is a test search result",
		Engine:        "google",
		Template:      "default.html",
		PublishedDate: &pubDate,
		Category:      "general",
		ImgSrc:        &imgSrc,
		Source:        &source,
	}

	if r.Title != "Test Result" {
		t.Errorf("Title = %q, want %q", r.Title, "Test Result")
	}
	if r.URL != "https://example.com/page" {
		t.Errorf("URL = %q, want %q", r.URL, "https://example.com/page")
	}
	if r.Content != "This is a test search result" {
		t.Errorf("Content = %q, want %q", r.Content, "This is a test search result")
	}
	if r.Engine != "google" {
		t.Errorf("Engine = %q, want %q", r.Engine, "google")
	}
	if r.Template != "default.html" {
		t.Errorf("Template = %q, want %q", r.Template, "default.html")
	}
	if r.PublishedDate == nil || *r.PublishedDate != "2024-01-15T10:00:00Z" {
		t.Errorf("PublishedDate = %v, want 2024-01-15T10:00:00Z", r.PublishedDate)
	}
	if r.Category != "general" {
		t.Errorf("Category = %q, want %q", r.Category, "general")
	}
	if r.ImgSrc == nil || *r.ImgSrc != "https://example.com/image.jpg" {
		t.Errorf("ImgSrc = %v, want https://example.com/image.jpg", r.ImgSrc)
	}
	if r.Source == nil || *r.Source != "example.com" {
		t.Errorf("Source = %v, want example.com", r.Source)
	}
}

func TestSearchResult_ZeroValues(t *testing.T) {
	t.Parallel()

	var r SearchResult

	if r.Title != "" {
		t.Errorf("Title = %q, want empty", r.Title)
	}
	if r.URL != "" {
		t.Errorf("URL = %q, want empty", r.URL)
	}
	if r.Content != "" {
		t.Errorf("Content = %q, want empty", r.Content)
	}
	if r.Engine != "" {
		t.Errorf("Engine = %q, want empty", r.Engine)
	}
	if r.Template != "" {
		t.Errorf("Template = %q, want empty", r.Template)
	}
	if r.PublishedDate != nil {
		t.Errorf("PublishedDate = %v, want nil", r.PublishedDate)
	}
	if r.Category != "" {
		t.Errorf("Category = %q, want empty", r.Category)
	}
	if r.ImgSrc != nil {
		t.Errorf("ImgSrc = %v, want nil", r.ImgSrc)
	}
	if r.Source != nil {
		t.Errorf("Source = %v, want nil", r.Source)
	}
}

func TestSearchResponse_Populate(t *testing.T) {
	t.Parallel()

	correctedURL := "https://example.com/corrected"

	resp := SearchResponse{
		Query: "test query",
		Results: []SearchResult{
			{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Engine: "google"},
		},
		Answers:         []AnswerResult{"42"},
		Suggestions:     []SuggestionResult{"suggestion 1"},
		NumberOfResults: 100,
		Paging:          true,
		CorrectedURL:    &correctedURL,
	}

	if resp.Query != "test query" {
		t.Errorf("Query = %q, want %q", resp.Query, "test query")
	}
	if len(resp.Results) != 1 {
		t.Errorf("len(Results) = %d, want 1", len(resp.Results))
	}
	if len(resp.Answers) != 1 || string(resp.Answers[0]) != "42" {
		t.Errorf("Answers = %v, want [42]", resp.Answers)
	}
	if len(resp.Suggestions) != 1 || string(resp.Suggestions[0]) != "suggestion 1" {
		t.Errorf("Suggestions = %v, want [suggestion 1]", resp.Suggestions)
	}
	if resp.NumberOfResults != 100 {
		t.Errorf("NumberOfResults = %d, want 100", resp.NumberOfResults)
	}
	if !resp.Paging {
		t.Errorf("Paging = false, want true")
	}
	if resp.CorrectedURL == nil || *resp.CorrectedURL != "https://example.com/corrected" {
		t.Errorf("CorrectedURL = %v, want https://example.com/corrected", resp.CorrectedURL)
	}
}

func TestSearchResponse_ZeroValues(t *testing.T) {
	t.Parallel()

	var resp SearchResponse

	if resp.Query != "" {
		t.Errorf("Query = %q, want empty", resp.Query)
	}
	if resp.Results != nil {
		t.Errorf("Results = %v, want nil", resp.Results)
	}
	if resp.Answers != nil {
		t.Errorf("Answers = %v, want nil", resp.Answers)
	}
	if resp.Suggestions != nil {
		t.Errorf("Suggestions = %v, want nil", resp.Suggestions)
	}
	if resp.CorrectedURL != nil {
		t.Errorf("CorrectedURL = %v, want nil", resp.CorrectedURL)
	}
}

func TestSearchParams_Populate(t *testing.T) {
	t.Parallel()

	p := SearchParams{
		Query:      "test search",
		Categories: []string{"general", "news"},
		Language:   "en-US",
		Page:       2,
		TimeRange:  "month",
		SafeSearch: 1,
	}

	if p.Query != "test search" {
		t.Errorf("Query = %q, want %q", p.Query, "test search")
	}
	if len(p.Categories) != 2 || p.Categories[0] != "general" || p.Categories[1] != "news" {
		t.Errorf("Categories = %v, want [general news]", p.Categories)
	}
	if p.Language != "en-US" {
		t.Errorf("Language = %q, want %q", p.Language, "en-US")
	}
	if p.Page != 2 {
		t.Errorf("Page = %d, want 2", p.Page)
	}
	if p.TimeRange != "month" {
		t.Errorf("TimeRange = %q, want %q", p.TimeRange, "month")
	}
	if p.SafeSearch != 1 {
		t.Errorf("SafeSearch = %d, want 1", p.SafeSearch)
	}
}

func TestSearchParams_ZeroValues(t *testing.T) {
	t.Parallel()

	var p SearchParams

	if p.Query != "" {
		t.Errorf("Query = %q, want empty", p.Query)
	}
	if p.Categories != nil {
		t.Errorf("Categories = %v, want nil", p.Categories)
	}
	if p.Language != "" {
		t.Errorf("Language = %q, want empty", p.Language)
	}
	if p.Page != 0 {
		t.Errorf("Page = %d, want 0", p.Page)
	}
	if p.TimeRange != "" {
		t.Errorf("TimeRange = %q, want empty", p.TimeRange)
	}
	if p.SafeSearch != 0 {
		t.Errorf("SafeSearch = %d, want 0", p.SafeSearch)
	}
}

func TestInfobox_Populate(t *testing.T) {
	t.Parallel()

	ib := Infobox{
		ID:      "infobox_1",
		URL:     "https://example.com/info",
		Content: "Infobox content",
		Infoboxes: []InfoboxDetail{
			{ID: "detail_1", Content: "Detail content"},
		},
		Engine: "duckduckgo",
		URLs: []InfoboxURL{
			{Title: "Link 1", URL: "https://example.com/1"},
		},
		Attributes: []InfoboxAttribute{
			{Key: "key1", Value: "value1"},
		},
	}

	if ib.ID != "infobox_1" {
		t.Errorf("ID = %q, want %q", ib.ID, "infobox_1")
	}
	if ib.URL != "https://example.com/info" {
		t.Errorf("URL = %q, want %q", ib.URL, "https://example.com/info")
	}
	if ib.Content != "Infobox content" {
		t.Errorf("Content = %q, want %q", ib.Content, "Infobox content")
	}
	if len(ib.Infoboxes) != 1 {
		t.Errorf("len(Infoboxes) = %d, want 1", len(ib.Infoboxes))
	}
	if ib.Engine != "duckduckgo" {
		t.Errorf("Engine = %q, want %q", ib.Engine, "duckduckgo")
	}
	if len(ib.URLs) != 1 || ib.URLs[0].Title != "Link 1" {
		t.Errorf("URLs = %v, want [Link 1]", ib.URLs)
	}
	if len(ib.Attributes) != 1 || ib.Attributes[0].Key != "key1" {
		t.Errorf("Attributes = %v, want [key1=value1]", ib.Attributes)
	}
}
