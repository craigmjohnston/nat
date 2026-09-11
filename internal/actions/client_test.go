package actions

import (
	"context"

	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
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

var _ store.API = (*fakeClient)(nil)

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

// The rest of [store.API] is here so a fakeClient can back a real
// [store.Notion], which is how these tests drive the plan reads and writes:
// what a launch or an approve does to a slice is asserted on the calls above,
// and nothing in either flow reaches any of the calls below.
func (f *fakeClient) GetDataSource(context.Context, string) (*notion.DataSource, error) {
	panic("not used")
}

func (f *fakeClient) QueryDataSource(context.Context, string, map[string]any, []notion.Sort) ([]notion.Page, error) {
	panic("not used")
}

func (f *fakeClient) UpdateDataSourceProperties(context.Context, string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
	panic("not used")
}

func (f *fakeClient) DataSourceOrder(context.Context, string) ([]string, error) { panic("not used") }

func (f *fakeClient) DeleteBlock(context.Context, string) error { panic("not used") }

func (f *fakeClient) CreatePage(context.Context, notion.Parent, map[string]notion.PropertyValue, []map[string]any) (*notion.Page, error) {
	panic("not used")
}

func (f *fakeClient) AppendBlockChildren(context.Context, string, []map[string]any) ([]notion.Block, error) {
	panic("not used")
}

func (f *fakeClient) TrashPage(context.Context, string) error { panic("not used") }

// store is the fake driving a real Notion-backed store, which is what the
// launch and approve flows now take.
func (f *fakeClient) store() Store { return store.Over(f) }
