package notion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// milestonesDSJSON and slicesDSJSON are the data sources a freshly created
// project reads back as — property ids and colours included, as Notion sends
// them.
const (
	milestonesDSJSON = `{
		"id":"ds-milestones",
		"name":"Milestones",
		"properties":{
			"Name":{"id":"title","name":"Name","type":"title","title":{}},
			"Order":{"id":"o","name":"Order","type":"number","number":{}},
			"Status":{"id":"s","name":"Status","type":"select","select":{"options":[
				{"id":"1","name":"Queued","color":"gray"},
				{"id":"2","name":"Active","color":"blue"},
				{"id":"3","name":"Done","color":"green"}
			]}}
		}
	}`

	// slicesDSJSON is the shape projects created before the app asked about an
	// assignee read back as: an Assignee column, and Claimed for in progress.
	slicesDSJSON = `{
		"id":"ds-slices",
		"name":"Slices",
		"properties":{
			"Name":{"id":"title","name":"Name","type":"title","title":{}},
			"Status":{"id":"s","name":"Status","type":"select","select":{"options":[
				{"id":"1","name":"Todo","color":"gray"},
				{"id":"2","name":"Claimed","color":"blue"},
				{"id":"3","name":"Done","color":"green"}
			]}},
			"Milestone":{"id":"m","name":"Milestone","type":"relation","relation":{"data_source_id":"DS-MILESTONES"}},
			"Assignee":{"id":"a","name":"Assignee","type":"people","people":{}},
			"Repo":{"id":"r","name":"Repo","type":"rich_text","rich_text":{}},
			"PR":{"id":"p","name":"PR","type":"url","url":{}}
		}
	}`

	// modernSlicesDSJSON is the shape a project created with the default
	// answer reads back as: no Assignee column, and In progress.
	modernSlicesDSJSON = `{
		"id":"ds-slices",
		"name":"Slices",
		"properties":{
			"Name":{"id":"title","name":"Name","type":"title","title":{}},
			"Status":{"id":"s","name":"Status","type":"select","select":{"options":[
				{"id":"1","name":"Todo","color":"gray"},
				{"id":"2","name":"In progress","color":"blue"},
				{"id":"3","name":"Done","color":"green"}
			]}},
			"Milestone":{"id":"m","name":"Milestone","type":"relation","relation":{"data_source_id":"DS-MILESTONES"}},
			"Repo":{"id":"r","name":"Repo","type":"rich_text","rich_text":{}},
			"PR":{"id":"p","name":"PR","type":"url","url":{}}
		}
	}`
)

// projectServer answers the whole create-a-project conversation: the project
// page, the two databases, then the schema read-backs. It records every
// request body in order.
func projectServer(t *testing.T, dataSources map[string]string) (*httptest.Server, *[]string) {
	t.Helper()
	var bodies []string
	dbIndex := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, r.Method+" "+r.URL.Path+" "+string(b))
		switch {
		case r.URL.Path == "/pages":
			w.Write([]byte(`{"id":"page-1","url":"https://notion.so/page-1"}`))
		case r.URL.Path == "/databases":
			dbIndex++
			if dbIndex == 1 {
				w.Write([]byte(`{"id":"db-milestones","data_sources":[{"id":"ds-milestones"}]}`))
				return
			}
			w.Write([]byte(`{"id":"db-slices","data_sources":[{"id":"ds-slices"}]}`))
		case strings.HasPrefix(r.URL.Path, "/data_sources/"):
			id := strings.TrimPrefix(r.URL.Path, "/data_sources/")
			body, ok := dataSources[id]
			if !ok {
				t.Errorf("unexpected data source fetch: %s", id)
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
				return
			}
			w.Write([]byte(body))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

func healthyDataSources() map[string]string {
	return map[string]string{"ds-milestones": milestonesDSJSON, "ds-slices": slicesDSJSON}
}

// modernDataSources is the same project in the shape CreateProject now makes:
// no Assignee column, and In progress where the older one says Claimed.
func modernDataSources() map[string]string {
	return map[string]string{"ds-milestones": milestonesDSJSON, "ds-slices": modernSlicesDSJSON}
}

func TestCreateProject(t *testing.T) {
	t.Run("creates page, milestones, slices, then verifies", func(t *testing.T) {
		srv, bodies := projectServer(t, modernDataSources())
		c, _ := testClient(t, srv)

		got, err := c.CreateProject(context.Background(), "ds-projects", "notion-agent-tracker", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := &ProjectStructure{
			PageID:         "page-1",
			PageURL:        "https://notion.so/page-1",
			MilestonesDBID: "db-milestones",
			MilestonesDSID: "ds-milestones",
			SlicesDBID:     "db-slices",
			SlicesDSID:     "ds-slices",
		}
		if *got != *want {
			t.Errorf("structure = %+v, want %+v", got, want)
		}

		wantBodies := []string{
			`POST /pages {"parent":{"type":"data_source_id","data_source_id":"ds-projects"},` +
				`"properties":{"Name":{"title":[{"type":"text","text":{"content":"notion-agent-tracker"}}]}}}`,
			`POST /databases {"initial_data_source":{"properties":{` +
				`"Name":{"title":{}},` +
				`"Order":{"number":{}},` +
				`"Status":{"select":{"options":[{"name":"Queued"},{"name":"Active"},{"name":"Done"}]}}}},` +
				`"parent":{"page_id":"page-1","type":"page_id"},` +
				`"title":[{"type":"text","text":{"content":"Milestones"}}]}`,
			`POST /databases {"initial_data_source":{"properties":{` +
				`"Milestone":{"relation":{"data_source_id":"ds-milestones","type":"single_property","single_property":{}}},` +
				`"Name":{"title":{}},` +
				`"PR":{"url":{}},` +
				`"Repo":{"rich_text":{}},` +
				`"Status":{"select":{"options":[{"name":"Todo"},{"name":"In progress"},{"name":"Done"}]}}}},` +
				`"parent":{"page_id":"page-1","type":"page_id"},` +
				`"title":[{"type":"text","text":{"content":"Slices"}}]}`,
			`GET /data_sources/ds-milestones `,
			`GET /data_sources/ds-slices `,
		}
		if len(*bodies) != len(wantBodies) {
			t.Fatalf("requests =\n%s\nwant %d requests", strings.Join(*bodies, "\n"), len(wantBodies))
		}
		for i, w := range wantBodies {
			if (*bodies)[i] != w {
				t.Errorf("request %d =\n%s\nwant\n%s", i, (*bodies)[i], w)
			}
		}
	})

	t.Run("propagates a page creation error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":"validation_error","message":"bad parent"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := c.CreateProject(context.Background(), "ds-projects", "p", false)
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got %v, want *APIError", err)
		}
		if !strings.Contains(err.Error(), "create project page") {
			t.Errorf("error = %v, want it to name the failing step", err)
		}
		if got != nil {
			t.Errorf("structure = %+v, want nil on error", got)
		}
	})

	t.Run("propagates a database creation error", func(t *testing.T) {
		var dbCalls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/pages" {
				w.Write([]byte(`{"id":"page-1"}`))
				return
			}
			dbCalls++
			if dbCalls == 1 {
				w.Write([]byte(`{"id":"db-milestones","data_sources":[{"id":"ds-milestones"}]}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"code":"validation_error","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := c.CreateProject(context.Background(), "ds-projects", "p", false)
		if err == nil || !strings.Contains(err.Error(), "create Slices database") {
			t.Fatalf("error = %v, want it to name the Slices database", err)
		}
		if got != nil {
			t.Errorf("structure = %+v, want nil on error", got)
		}
	})

	t.Run("reports a database created without a data source", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/pages" {
				w.Write([]byte(`{"id":"page-1"}`))
				return
			}
			w.Write([]byte(`{"id":"db-milestones"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		got, err := c.CreateProject(context.Background(), "ds-projects", "p", false)
		if err == nil || !strings.Contains(err.Error(), "no data source") {
			t.Fatalf("error = %v, want a missing data source error", err)
		}
		if got != nil {
			t.Errorf("structure = %+v, want nil on error", got)
		}
	})

	t.Run("returns what it created when verification fails", func(t *testing.T) {
		ds := healthyDataSources()
		ds["ds-slices"] = `{"id":"ds-slices","properties":{}}`
		srv, _ := projectServer(t, ds)
		c, _ := testClient(t, srv)

		got, err := c.CreateProject(context.Background(), "ds-projects", "p", false)
		var schemaErr *SchemaError
		if !errors.As(err, &schemaErr) {
			t.Fatalf("got %v, want *SchemaError", err)
		}
		if got == nil || got.SlicesDSID != "ds-slices" {
			t.Fatalf("structure = %+v, want the created ids alongside the error", got)
		}
	})
}

func TestCreateProjectsDatabase(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{"id":"db-projects","data_sources":[{"id":"ds-projects"}]}`))
	}))
	defer srv.Close()

	c, _ := testClient(t, srv)
	db, err := c.CreateProjectsDatabase(context.Background(), "page-1", "Agent Projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"initial_data_source":{"properties":{"Name":{"title":{}}}},` +
		`"parent":{"page_id":"page-1","type":"page_id"},` +
		`"title":[{"type":"text","text":{"content":"Agent Projects"}}]}`
	if gotBody != want {
		t.Errorf("request body =\n%s\nwant\n%s", gotBody, want)
	}
	if id, ok := db.DataSourceID(); !ok || id != "ds-projects" {
		t.Errorf("DataSourceID() = %q, %v", id, ok)
	}
}

func TestVerifyProjectSchema(t *testing.T) {
	t.Run("accepts a healthy project", func(t *testing.T) {
		srv, _ := projectServer(t, healthyDataSources())
		c, _ := testClient(t, srv)

		// The relation reads back dashless and upper-cased: ids compare
		// loosely, as the bootstrap project's do.
		if err := c.VerifyProjectSchema(context.Background(), "ds-milestones", "ds-slices"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("accepts a project created without an assignee", func(t *testing.T) {
		srv, _ := projectServer(t, modernDataSources())
		c, _ := testClient(t, srv)

		if err := c.VerifyProjectSchema(context.Background(), "ds-milestones", "ds-slices"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("reports every mismatch at once", func(t *testing.T) {
		ds := healthyDataSources()
		ds["ds-slices"] = `{
			"id":"ds-slices",
			"properties":{
				"Name":{"type":"title","title":{}},
				"Status":{"type":"select","select":{"options":[{"name":"Todo"}]}},
				"Milestone":{"type":"relation","relation":{"data_source_id":"ds-somewhere-else"}},
				"Repo":{"type":"url","url":{}}
			}
		}`
		srv, _ := projectServer(t, ds)
		c, _ := testClient(t, srv)

		err := c.VerifyProjectSchema(context.Background(), "ds-milestones", "ds-slices")
		var schemaErr *SchemaError
		if !errors.As(err, &schemaErr) {
			t.Fatalf("got %v, want *SchemaError", err)
		}
		if schemaErr.DataSource != SlicesDBTitle {
			t.Errorf("data source = %q, want %q", schemaErr.DataSource, SlicesDBTitle)
		}
		want := []string{
			`property "Status" is missing option "Done"`,
			`property "Status" offers none of the options "In progress" or "Claimed"`,
			`property "Milestone" does not relate to data source ds-milestones`,
			`property "Repo" is a url, want rich_text`,
			`missing property "PR" (url)`,
		}
		if len(schemaErr.Problems) != len(want) {
			t.Fatalf("problems = %v, want %d", schemaErr.Problems, len(want))
		}
		for i, w := range want {
			if schemaErr.Problems[i] != w {
				t.Errorf("problem %d = %q, want %q", i, schemaErr.Problems[i], w)
			}
		}
		if msg := err.Error(); !strings.HasPrefix(msg, "Slices schema: ") || !strings.Contains(msg, "; ") {
			t.Errorf("Error() = %q, want the problems joined under the data source name", msg)
		}
	})

	t.Run("reports mismatches in both data sources", func(t *testing.T) {
		ds := healthyDataSources()
		ds["ds-milestones"] = `{"id":"ds-milestones","properties":{}}`
		ds["ds-slices"] = `{"id":"ds-slices","properties":{}}`
		srv, _ := projectServer(t, ds)
		c, _ := testClient(t, srv)

		err := c.VerifyProjectSchema(context.Background(), "ds-milestones", "ds-slices")
		if err == nil {
			t.Fatal("expected an error")
		}
		for _, want := range []string{"Milestones schema:", "Slices schema:", `missing property "Order"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})

	t.Run("propagates a fetch error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":"object_not_found","message":"nope"}`))
		}))
		defer srv.Close()

		c, _ := testClient(t, srv)
		err := c.VerifyProjectSchema(context.Background(), "ds-milestones", "ds-slices")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.NotFound() {
			t.Fatalf("got %v, want a not-found *APIError", err)
		}
		if !strings.Contains(err.Error(), "verify Milestones schema") {
			t.Errorf("error = %v, want it to name the data source", err)
		}
	})
}

func TestProjectSchemas(t *testing.T) {
	// The schemas are what CreateProject sends; onboarding and the new-project
	// flow reuse them, so pin their shape.
	tests := []struct {
		name   string
		schema map[string]PropertySchema
		want   string
	}{
		{
			"milestones",
			MilestonesSchema(),
			`{"Name":{"title":{}},"Order":{"number":{}},` +
				`"Status":{"select":{"options":[{"name":"Queued"},{"name":"Active"},{"name":"Done"}]}}}`,
		},
		{
			"slices",
			SlicesSchema("ds-1", false),
			`{"Milestone":{"relation":{"data_source_id":"ds-1","type":"single_property","single_property":{}}},` +
				`"Name":{"title":{}},"PR":{"url":{}},"Repo":{"rich_text":{}},` +
				`"Status":{"select":{"options":[{"name":"Todo"},{"name":"In progress"},{"name":"Done"}]}}}`,
		},
		{
			"slices with an assignee",
			SlicesSchema("ds-1", true),
			`{"Assignee":{"people":{}},"Milestone":{"relation":{"data_source_id":"ds-1","type":"single_property","single_property":{}}},` +
				`"Name":{"title":{}},"PR":{"url":{}},"Repo":{"rich_text":{}},` +
				`"Status":{"select":{"options":[{"name":"Todo"},{"name":"In progress"},{"name":"Done"}]}}}`,
		},
		{"projects", ProjectsSchema(), `{"Name":{"title":{}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.schema)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(b) != tt.want {
				t.Errorf("marshalled to\n%s\nwant\n%s", b, tt.want)
			}
		})
	}
}

func TestSameID(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"3b738308-f654-8111-966b-e79b7d626133", "3b738308f6548111966be79b7d626133", true},
		{"ABC-DEF", "abcdef", true},
		{"ds-1", "ds-2", false},
		{"", "", true},
	}
	for _, tt := range tests {
		if got := sameID(tt.a, tt.b); got != tt.want {
			t.Errorf("sameID(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestShapeOf(t *testing.T) {
	tests := []struct {
		name string
		ds   DataSource
		want SliceShape
	}{
		{
			"a project created before the question was asked",
			DataSource{Properties: map[string]PropertySchema{
				PropStatus:   {Type: TypeSelect, Select: &OptionsConfig{Options: selectOptions([]string{SliceTodo, SliceClaimed, SliceDone})}},
				PropAssignee: {Type: TypePeople},
			}},
			SliceShape{InProgress: SliceClaimed, StatusType: TypeSelect, HasAssignee: true},
		},
		{
			"a project created with the new default",
			DataSource{Properties: map[string]PropertySchema{
				PropStatus: {Type: TypeSelect, Select: &OptionsConfig{Options: selectOptions([]string{SliceTodo, SliceInProgress, SliceDone})}},
			}},
			SliceShape{InProgress: SliceInProgress, StatusType: TypeSelect},
		},
		{
			"a Status column converted to Notion's status type",
			DataSource{Properties: map[string]PropertySchema{
				PropStatus: {Type: TypeStatus, Status: &OptionsConfig{Options: selectOptions([]string{SliceTodo, SliceInProgress, SliceDone})}},
			}},
			SliceShape{InProgress: SliceInProgress, StatusType: TypeStatus},
		},
		{
			"options that cannot be read fall back to Claimed",
			DataSource{Properties: map[string]PropertySchema{PropStatus: {Type: TypeStatus}}},
			SliceShape{InProgress: SliceClaimed, StatusType: TypeStatus},
		},
		{
			"an Assignee column of another type is not one to write to",
			DataSource{Properties: map[string]PropertySchema{
				PropStatus:   {Type: TypeSelect, Select: &OptionsConfig{Options: selectOptions([]string{SliceInProgress})}},
				PropAssignee: {Type: "rich_text"},
			}},
			SliceShape{InProgress: SliceInProgress, StatusType: TypeSelect},
		},
		{"an empty data source", DataSource{}, SliceShape{InProgress: SliceClaimed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShapeOf(&tt.ds); got != tt.want {
				t.Errorf("ShapeOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
