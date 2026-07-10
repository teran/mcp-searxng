package searxng

import (
	"encoding/json"

	"github.com/teran/mcp-searxng/domain"
)

// raw models — JSON representation matching SearXNG Search API wire format.

type rawSearchResponse struct {
	Query           string            `json:"query"`
	Results         []rawSearchResult `json:"results"`
	Answers         []json.RawMessage `json:"answers,omitempty"`
	Infoboxes       []rawInfobox      `json:"infoboxes,omitempty"`
	Suggestions     []string          `json:"suggestions,omitempty"`
	NumberOfResults int               `json:"number_of_results,omitempty"`
	Paging          bool              `json:"paging,omitempty"`
}

type rawSearchResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Content       string   `json:"content"`
	Engine        string   `json:"engine"`
	Template      string   `json:"template,omitempty"`
	PublishedDate *string  `json:"publishedDate,omitempty"`
	Category      string   `json:"category,omitempty"`
	ImgSrc        *string  `json:"img_src,omitempty"`
	Source        *string  `json:"source,omitempty"`
	EngineAvatar  *string  `json:"engine_avatar,omitempty"`
	ParsedURL     []string `json:"parsed_url,omitempty"`
}

type rawInfobox struct {
	ID         string                `json:"id,omitempty"`
	URL        string                `json:"url,omitempty"`
	Content    string                `json:"content,omitempty"`
	Infoboxes  []rawInfoboxDetail    `json:"infoboxes,omitempty"`
	ImgSrc     *string               `json:"img_src,omitempty"`
	Engine     string                `json:"engine,omitempty"`
	URLs       []rawInfoboxURL       `json:"urls,omitempty"`
	Attributes []rawInfoboxAttribute `json:"attributes,omitempty"`
}

type rawInfoboxURL struct {
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

type rawInfoboxAttribute struct {
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

type rawInfoboxDetail struct {
	ID         string                `json:"id,omitempty"`
	Content    string                `json:"content,omitempty"`
	ImgSrc     *string               `json:"img_src,omitempty"`
	URLs       []rawInfoboxURL       `json:"urls,omitempty"`
	Attributes []rawInfoboxAttribute `json:"attributes,omitempty"`
}

// -- domain conversion --

func (r rawSearchResponse) toDomain() domain.SearchResponse {
	results := make([]domain.SearchResult, 0, len(r.Results))
	for _, res := range r.Results {
		results = append(results, res.toDomain())
	}

	infoboxes := make([]domain.Infobox, 0, len(r.Infoboxes))
	for _, ib := range r.Infoboxes {
		infoboxes = append(infoboxes, ib.toDomain())
	}

	suggestions := make([]domain.SuggestionResult, 0, len(r.Suggestions))
	for _, s := range r.Suggestions {
		suggestions = append(suggestions, domain.SuggestionResult(s))
	}

	answers := make([]domain.AnswerResult, 0, len(r.Answers))
	for _, a := range r.Answers {
		answers = append(answers, rawAnswerToString(a))
	}

	return domain.SearchResponse{
		Query:           r.Query,
		Results:         results,
		Answers:         answers,
		Infoboxes:       infoboxes,
		Suggestions:     suggestions,
		NumberOfResults: r.NumberOfResults,
		Paging:          r.Paging,
	}
}

func (r rawSearchResult) toDomain() domain.SearchResult {
	return domain.SearchResult{
		Title:         r.Title,
		URL:           r.URL,
		Content:       r.Content,
		Engine:        r.Engine,
		Template:      r.Template,
		PublishedDate: r.PublishedDate,
		Category:      r.Category,
		ImgSrc:        r.ImgSrc,
		Source:        r.Source,
		EngineAvatar:  r.EngineAvatar,
		ParsedURL:     r.ParsedURL,
	}
}

func (r rawInfobox) toDomain() domain.Infobox {
	details := make([]domain.InfoboxDetail, 0, len(r.Infoboxes))
	for _, d := range r.Infoboxes {
		details = append(details, d.toDomain())
	}

	urls := make([]domain.InfoboxURL, 0, len(r.URLs))
	for _, u := range r.URLs {
		urls = append(urls, u.toDomain())
	}

	attrs := make([]domain.InfoboxAttribute, 0, len(r.Attributes))
	for _, a := range r.Attributes {
		attrs = append(attrs, a.toDomain())
	}

	return domain.Infobox{
		ID:         r.ID,
		URL:        r.URL,
		Content:    r.Content,
		Infoboxes:  details,
		ImgSrc:     r.ImgSrc,
		Engine:     r.Engine,
		URLs:       urls,
		Attributes: attrs,
	}
}

func (r rawInfoboxURL) toDomain() domain.InfoboxURL {
	return domain.InfoboxURL{
		Title: r.Title,
		URL:   r.URL,
	}
}

func (r rawInfoboxAttribute) toDomain() domain.InfoboxAttribute {
	return domain.InfoboxAttribute{
		Key:   r.Key,
		Value: r.Value,
	}
}

// rawAnswerToString converts a json.RawMessage answer to domain.AnswerResult.
// It handles both plain strings and objects by JSON-marshaling objects back to string.
func rawAnswerToString(a json.RawMessage) domain.AnswerResult {
	var s string
	if err := json.Unmarshal(a, &s); err == nil {
		return domain.AnswerResult(s)
	}
	// If it's not a plain string (e.g. an object), marshal it back to JSON string.
	b, err := json.Marshal(a)
	if err != nil {
		return domain.AnswerResult(string(a))
	}
	return domain.AnswerResult(string(b))
}

func (r rawInfoboxDetail) toDomain() domain.InfoboxDetail {
	urls := make([]domain.InfoboxURL, 0, len(r.URLs))
	for _, u := range r.URLs {
		urls = append(urls, u.toDomain())
	}

	attrs := make([]domain.InfoboxAttribute, 0, len(r.Attributes))
	for _, a := range r.Attributes {
		attrs = append(attrs, a.toDomain())
	}

	return domain.InfoboxDetail{
		ID:         r.ID,
		Content:    r.Content,
		ImgSrc:     r.ImgSrc,
		URLs:       urls,
		Attributes: attrs,
	}
}
