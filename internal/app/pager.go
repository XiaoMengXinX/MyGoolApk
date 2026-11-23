package app

import (
	"context"
	"strconv"

	"github.com/nsgo-dev/nsgo/core"
)

// pager loads one paginated list into signals. Pages are 1-based. A failed
// first page leaves err set while nothing is loaded; a failed refresh keeps
// the items and sets err, which the list shows as a banner. moreErr stops
// automatic paging until retryMore.
type pager[T any] struct {
	cx *core.Scope

	items      *core.Signal[[]T]
	loaded     *core.Signal[bool]
	hasMore    *core.Signal[bool]
	refreshing *core.Signal[bool]
	err        *core.Signal[string]
	moreErr    *core.Signal[string]

	page    int
	loading bool
	more    bool
	gen     int
	fetch   func(ctx context.Context, page int) ([]T, bool, error)
	id      func(T) string
	list    core.Ref
	started bool
}

func newPager[T any](cx *core.Scope, fetch func(context.Context, int) ([]T, bool, error), id func(T) string) *pager[T] {
	return &pager[T]{
		cx:         cx,
		items:      core.NewSignalFunc(cx, []T(nil), nil),
		loaded:     core.NewSignal(cx, false),
		hasMore:    core.NewSignal(cx, false),
		refreshing: core.NewSignal(cx, false),
		err:        core.NewSignal(cx, ""),
		moreErr:    core.NewSignal(cx, ""),
		fetch:      fetch,
		id:         id,
	}
}

// ensure loads the first page the first time a list is shown.
func (p *pager[T]) ensure() {
	if !p.started {
		p.started = true
		p.load(false)
	}
}

func (p *pager[T]) reload() { p.load(false) }

// restart loads the first page again even while one is on the way, as after
// its source changed: the answer to the old one is dropped.
func (p *pager[T]) restart() {
	p.loading, p.more = false, false
	p.load(false)
}

func (p *pager[T]) retry() {
	p.err.Set("")
	p.load(false)
}

func (p *pager[T]) loadMore() {
	if p.loaded.GetUntracked() && p.hasMore.GetUntracked() && p.moreErr.GetUntracked() == "" {
		p.load(true)
	}
}

func (p *pager[T]) retryMore() {
	p.moreErr.Set("")
	p.loadMore()
}

// reset drops what was loaded, as a new source asks, and loads again.
func (p *pager[T]) reset() {
	p.gen++
	p.items.Set(nil)
	p.page = 0
	p.loading, p.more, p.started = false, false, false
	p.loaded.Set(false)
	p.hasMore.Set(false)
	p.refreshing.Set(false)
	p.err.Set("")
	p.moreErr.Set("")
	p.ensure()
}

func (p *pager[T]) load(more bool) {
	// A first page waits for none: it drops a next page still on the way,
	// whose answer the new generation ignores.
	if p.loading || (more && p.more) {
		return
	}
	p.more = false
	page := 1
	if more {
		page = p.page + 1
		p.more = true
	} else {
		p.loading = true
	}
	p.gen++
	gen := p.gen
	go func() {
		items, hasMore, err := p.fetch(context.Background(), page)
		p.cx.Post(func() {
			if gen != p.gen {
				return
			}
			p.loading, p.more = false, false
			p.refreshing.Set(false)
			switch {
			case err != nil && more:
				p.moreErr.Set(err.Error())
			case err != nil && p.loaded.GetUntracked():
				p.err.Set("刷新失败：" + err.Error())
			case err != nil:
				p.err.Set(err.Error())
			default:
				var base []T
				if more {
					base = p.items.GetUntracked()
				}
				p.items.Set(mergeByID(base, items, p.id))
				p.page = page
				p.hasMore.Set(hasMore)
				p.loaded.Set(true)
				p.err.Set("")
				p.moreErr.Set("")
			}
		})
	}()
}

// at returns item i, and whether there is one.
func (p *pager[T]) at(i int) (T, bool) {
	items := p.items.Get()
	if i < 0 || i >= len(items) {
		var zero T
		return zero, false
	}
	return items[i], true
}

// find returns the loaded item with key.
func (p *pager[T]) find(key string) (T, bool) {
	for _, it := range p.items.GetUntracked() {
		if p.id(it) == key {
			return it, true
		}
	}
	var zero T
	return zero, false
}

// update replaces the loaded item with key by fn of it.
func (p *pager[T]) update(key string, fn func(T) T) {
	items := append([]T(nil), p.items.GetUntracked()...)
	for i, it := range items {
		if p.id(it) == key {
			items[i] = fn(it)
			p.items.Set(items)
			return
		}
	}
}

// mergeByID appends more to base, dropping items whose non-empty ID is
// already present: Coolapk pages overlap when new posts arrive.
func mergeByID[T any](base, more []T, id func(T) string) []T {
	seen := make(map[string]bool, len(base)+len(more))
	out := make([]T, 0, len(base)+len(more))
	for _, list := range [2][]T{base, more} {
		for _, item := range list {
			if key := id(item); key != "" {
				if seen[key] {
					continue
				}
				seen[key] = true
			}
			out = append(out, item)
		}
	}
	return out
}

// rowKind is what a list row shows.
type rowKind int

const (
	rowItem rowKind = iota
	rowLoading
	rowFailed
	rowEmpty
	rowFooter
)

// listSpec describes the rows around a pager's items.
type listSpec struct {
	empty     string
	emptyHint string
	// skeleton is the loading row; nil shows a spinner.
	skeleton func() core.Node
	// header is shown above the rows.
	header core.Node
	// padding below the last row.
	bottom core.Points
	// pushed says the list is on a page pushed over the shell, which draws under
	// the system bars.
	pushed bool
	// onRefresh runs on a pull to refresh; nil reloads the pager.
	onRefresh func()
}

// pagerList shows a pager as a virtual list with its loading, empty, error
// and paging states. item builds the content of a row: get reads the item
// the row shows now, so the content updates as the list reuses the row.
func pagerList[T any](cx *core.Scope, p *pager[T], spec listSpec, item func(cx *core.Scope, get func() T, row core.ListRow) core.Node) core.Node {
	p.ensure()
	count := func() int {
		switch {
		case !p.loaded.Get():
			return 3
		case len(p.items.Get()) == 0:
			return 1
		}
		return len(p.items.Get()) + 1
	}
	kind := func(i int) rowKind {
		switch {
		case !p.loaded.Get() && p.err.Get() != "":
			return rowFailed
		case !p.loaded.Get():
			return rowLoading
		case len(p.items.Get()) == 0:
			return rowEmpty
		case i < len(p.items.Get()):
			return rowItem
		}
		return rowFooter
	}
	banner := core.Dynamic(func(cx *core.Scope) core.Node {
		if !p.loaded.Get() || p.err.Get() == "" {
			return core.Group{}
		}
		return errorBanner(p.err.Get(), p.reload, func() { p.err.Set("") })
	})
	header := []core.Node{banner}
	if spec.header != nil {
		header = append(header, spec.header)
	}
	return core.List{
		Ref:           &p.list,
		TestID:        "list",
		Count:         count,
		EstimatedSize: 220,
		Header:        core.Group(header),
		Footer:        bottomSpace(spec.bottom, spec.pushed),
		Key: func(i int) any {
			switch kind(i) {
			case rowItem:
				if it, ok := p.at(i); ok {
					if key := p.id(it); key != "" {
						return key
					}
				}
				return i
			case rowLoading:
				return "loading:" + strconv.Itoa(i)
			}
			return kind(i)
		},
		Row: func(cx *core.Scope, r core.ListRow) core.Node {
			k := core.NewMemo(cx, func() rowKind { return kind(r.Index()) })
			get := func() T { it, _ := p.at(r.Index()); return it }
			return core.Dynamic(func(cx *core.Scope) core.Node {
				switch k.Get() {
				case rowItem:
					return item(cx, get, r)
				case rowLoading:
					if spec.skeleton != nil {
						return spec.skeleton()
					}
					return column(core.Style{Padding: core.All(32), AlignItems: core.AlignCenter}, "", spinner())
				case rowFailed:
					return statusBlock("加载失败", p.err.Get(), "重试", p.retry)
				case rowEmpty:
					return statusBlock(spec.empty, spec.emptyHint, "刷新", p.reload)
				}
				return pagerFooter(cx, p)
			})
		},
		OnLoadMore:        p.loadMore,
		LoadMoreThreshold: 1.5,
		OnRefresh: func() {
			p.refreshing.Set(true)
			if spec.onRefresh != nil {
				spec.onRefresh()
			} else {
				p.reload()
			}
		},
		Refreshing: p.refreshing.Get,
		Style:      fill,
	}
}

// pagerFooter ends the list: a spinner while more is coming, a retry after
// a failed page, a note at the end.
func pagerFooter[T any](cx *core.Scope, p *pager[T]) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		switch {
		case p.moreErr.Get() != "":
			return column(core.Style{Padding: core.All(16), Gap: 8, AlignItems: core.AlignCenter}, "",
				core.Text{Content: p.moreErr.Get(), Class: "text-on-surface-variant", MaxLines: 2, Style: textStyle(textCaption, 400)},
				textButton("重试", p.retryMore))
		case p.hasMore.Get():
			return column(core.Style{Padding: core.All(16), AlignItems: core.AlignCenter}, "", spinner())
		}
		return column(core.Style{Padding: core.All(16), AlignItems: core.AlignCenter}, "", label("没有更多了", textCaption, 400, "text-on-surface-variant"))
	})
}
