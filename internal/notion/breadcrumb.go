package notion

import "context"

// MaxBreadcrumbDepth caps how far Breadcrumb climbs. A trail is context for one
// row of a picker, so the nearest few ancestors are all that can usefully be
// drawn — and the cap is what stops a workspace whose parent chain does not
// terminate from costing an unbounded number of requests.
const MaxBreadcrumbDepth = 6

// BreadcrumbEllipsis stands in for the ancestors a trail does not name: the
// ones beyond the depth cap, and the ones a failed fetch put out of reach. A
// breadcrumb is decoration, so an unreachable parent degrades the trail rather
// than failing the call it was resolved for.
const BreadcrumbEllipsis = "…"

// Breadcrumb names the ancestors above parent, outermost first, by climbing the
// chain one object at a time: a database to the page holding it, a page to
// whatever holds that, and a block through to the page it is drawn on. It costs
// one request per step, so callers should resolve only the rows they are about
// to draw.
//
// It never fails. An ancestor that cannot be fetched, and one past the depth
// cap, become BreadcrumbEllipsis at the front of the trail, which is as much as
// a caller could do with the error anyway. Untitled ancestors and blocks
// contribute no segment of their own.
func (c *Client) Breadcrumb(ctx context.Context, parent Parent) []string {
	var trail []string
	for depth := 0; ; depth++ {
		if depth == MaxBreadcrumbDepth {
			trail = append(trail, BreadcrumbEllipsis)
			break
		}
		title, up, more, err := c.parentStep(ctx, parent)
		if err != nil {
			trail = append(trail, BreadcrumbEllipsis)
			break
		}
		if !more {
			break
		}
		if title != "" {
			trail = append(trail, title)
		}
		parent = up
	}
	// The walk collected the trail nearest-first; it reads the other way round.
	for i, j := 0, len(trail)-1; i < j; i, j = i+1, j-1 {
		trail[i], trail[j] = trail[j], trail[i]
	}
	return trail
}

// parentStep fetches the one object parent names, reporting its title and the
// parent above it. more is false where the chain ends: at the workspace, and at
// any parent this app has no way to climb past.
func (c *Client) parentStep(ctx context.Context, parent Parent) (title string, up Parent, more bool, err error) {
	switch parent.Type {
	case ParentDatabase:
		db, err := c.GetDatabase(ctx, parent.DatabaseID)
		if err != nil {
			return "", Parent{}, false, err
		}
		return db.TitleText(), db.Parent, true, nil
	case ParentPage:
		page, err := c.GetPage(ctx, parent.PageID)
		if err != nil {
			return "", Parent{}, false, err
		}
		return page.TitleText(), page.Parent, true, nil
	case ParentBlock:
		block, err := c.GetBlock(ctx, parent.BlockID)
		if err != nil {
			return "", Parent{}, false, err
		}
		// A block is a step through rather than a step of the trail: the user
		// thinks in pages, and a column has no name to show them.
		return "", block.Parent, true, nil
	}
	return "", Parent{}, false, nil
}
