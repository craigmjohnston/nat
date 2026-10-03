package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/craigmjohnston/nat/plugins/shortcut/internal/cache"
	"github.com/craigmjohnston/nat/plugins/shortcut/internal/shortcut"
)

// epicsKey is the long cache's key for the slim epic list.
const epicsKey = "epics"

// warmingKey marks a warm-up started, so a sidebar read again while one runs
// does not start another; warmingTTL is how long that mark is trusted, after
// which a warm-up that died is simply started again.
const (
	warmingKey = "warming"
	warmingTTL = time.Minute
)

// warmBudget caps the warm-up's one request. It runs detached, past nat's
// 20 s kill of the sidebar that started it, so it can wait on a list that
// takes seconds where a method could not.
var warmBudget = 2 * time.Minute

// cachedEpics is the epic list the filter editor offers, as the long cache
// holds it, and whether it is still to come. The list is never fetched on the
// sidebar's own path — even without descriptions it takes seconds on a real
// workspace, and the sidebar is what a project opening waits on — so where
// the cache has nothing, or only a stale list, a warm-up is started in the
// background (startWarm) and the stale list, if any, is offered meanwhile. A
// sidebar drawn with no list at all is kept out of the response cache, so the
// read gnat makes again finds the list as soon as the warm-up lands it.
func (a *app) cachedEpics() ([]shortcut.Epic, bool) {
	body, fresh, ok := a.refCache().Get(a.refScope(), epicsKey)
	if !fresh {
		a.startWarm()
	}
	if !ok {
		a.noCache = true
		return nil, true
	}
	var epics []shortcut.Epic
	// Our own marshalled list: it decodes.
	_ = json.Unmarshal(body, &epics)
	return epics, false
}

// startWarm starts `nat-source-shortcut warm` detached, unless one was started
// within warmingTTL. A start that fails marks nothing, so the next sidebar
// tries again.
func (a *app) startWarm() {
	if a.env.Spawn == nil {
		return
	}
	marks := cache.Cache{Dir: a.cache.Dir, TTL: warmingTTL, Now: a.env.Now}
	if _, fresh, _ := marks.Get(a.refScope(), warmingKey); fresh {
		return
	}
	if a.env.Spawn("warm") == nil {
		marks.Put(a.refScope(), warmingKey, []byte("true"))
	}
}

// warm is the background half of cachedEpics: run detached by a sidebar, it
// fetches the slim epic list into the long cache and exits. It says nothing —
// its stdio goes nowhere, and a failure is simply no list, which a later
// sidebar starts another warm-up for — so neither the token nor a body can
// reach anyone. Exit 0 with the list cached, 1 without.
func warm(env Env) int {
	tok := env.Getenv("SHORTCUT_API_TOKEN")
	if tok == "" {
		tok, _ = env.Tokens.Token()
	}
	if tok == "" {
		return 1
	}
	a := newApp(env, request{}, tok)
	if env.HTTP == nil {
		a.sc.HTTP = &http.Client{Timeout: warmBudget}
	}
	ctx, cancel := context.WithTimeout(context.Background(), warmBudget)
	defer cancel()
	epics, err := a.sc.Epics(ctx)
	if err != nil {
		return 1
	}
	a.refCache().Put(a.refScope(), epicsKey, marshal(epics))
	return 0
}
