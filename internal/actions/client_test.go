package actions

import (
	"context"

	"github.com/craigmjohnston/nat/internal/notion"
)

// updateCall is one write a fakeClient recorded, the way the board's own
// fakeNotion does.
type updateCall struct {
	pageID     string
	properties map[string]notion.PropertyValue
}

// fakeClient stands in for Notion: each call is a field, so a test supplies
// only the behaviour it cares about. Unset calls answer with a bare value
// carrying the ID asked for.
type fakeClient struct {
	getPage    func(id string) (*notion.Page, error)
	updatePage func(id string, properties map[string]notion.PropertyValue) (*notion.Page, error)
	blocks     func(id string) ([]notion.Block, error)

	fetchedPages []string
	updated      []updateCall
	blockParents []string
}

var _ Client = (*fakeClient)(nil)

func (f *fakeClient) GetPage(_ context.Context, id string) (*notion.Page, error) {
	f.fetchedPages = append(f.fetchedPages, id)
	if f.getPage == nil {
		return &notion.Page{ID: id}, nil
	}
	return f.getPage(id)
}

func (f *fakeClient) UpdatePageProperties(_ context.Context, pageID string, properties map[string]notion.PropertyValue) (*notion.Page, error) {
	f.updated = append(f.updated, updateCall{pageID: pageID, properties: properties})
	if f.updatePage == nil {
		return &notion.Page{ID: pageID}, nil
	}
	return f.updatePage(pageID, properties)
}

func (f *fakeClient) GetBlockChildren(_ context.Context, id string) ([]notion.Block, error) {
	f.blockParents = append(f.blockParents, id)
	if f.blocks == nil {
		return nil, nil
	}
	return f.blocks(id)
}
