package domain

// SearchResult represents a single search result from SearXNG.
type SearchResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	Engine        string  `json:"engine"`
	Template      string  `json:"template,omitempty"`
	PublishedDate *string `json:"publishedDate,omitempty"`
	Category      string  `json:"category,omitempty"`
	ImgSrc        *string `json:"img_src,omitempty"`
	Source        *string `json:"source,omitempty"`
	EngineAvatar  *string `json:"engine_avatar,omitempty"`
	ParsedURL     *string `json:"parsed_url,omitempty"`
}

// Infobox represents an infobox result from SearXNG.
type Infobox struct {
	ID         string             `json:"id,omitempty"`
	URL        string             `json:"url,omitempty"`
	Content    string             `json:"content,omitempty"`
	Infoboxes  []InfoboxDetail    `json:"infoboxes,omitempty"`
	ImgSrc     *string            `json:"img_src,omitempty"`
	Engine     string             `json:"engine,omitempty"`
	URLs       []InfoboxURL       `json:"urls,omitempty"`
	Attributes []InfoboxAttribute `json:"attributes,omitempty"`
}

// InfoboxURL represents a URL entry in an infobox.
type InfoboxURL struct {
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

// InfoboxAttribute represents an attribute in an infobox.
type InfoboxAttribute struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// InfoboxDetail contains detailed infobox data.
type InfoboxDetail struct {
	ID         string             `json:"id,omitempty"`
	Content    string             `json:"content,omitempty"`
	ImgSrc     *string            `json:"img_src,omitempty"`
	URLs       []InfoboxURL       `json:"urls,omitempty"`
	Attributes []InfoboxAttribute `json:"attributes,omitempty"`
}

// SuggestionResult represents a search suggestion.
type SuggestionResult string

// AnswerResult represents an answer result.
type AnswerResult string

// SearchResponse is the top-level response from the SearXNG search API.
type SearchResponse struct {
	Query           string             `json:"query"`
	Results         []SearchResult     `json:"results"`
	Answers         []AnswerResult     `json:"answers,omitempty"`
	Infoboxes       []Infobox          `json:"infoboxes,omitempty"`
	Suggestions     []SuggestionResult `json:"suggestions,omitempty"`
	NumberOfResults int                `json:"number_of_results,omitempty"`
	Paging          bool               `json:"paging,omitempty"`
	CorrectedURL    *string            `json:"corrected_url,omitempty"`
}
