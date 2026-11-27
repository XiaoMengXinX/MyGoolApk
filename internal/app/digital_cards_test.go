package app

import (
	"context"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestDigitalRankingAlignsHeat(t *testing.T) {
	card := &backend.Card{Kind: backend.CardRanking, Items: []backend.CardItem{
		{Title: "耳机", Hot: "3150"},
		{Title: "酷态科10号超级电能充Ultra", Hot: "1.2万"},
	}}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a := &App{cx: cx, images: newImageStore(t.TempDir(), "")}
		return a.rankingCard(cx, card)
	}, nsgotest.Size(400, 600))
	short := app.Find("text:3150").Bounds()
	long := app.Find("text:1.2万").Bounds()
	if short.Origin.X == long.Origin.X || short.Origin.Y != long.Origin.Y {
		t.Fatalf("heat positions are not on the same row: %v and %v", short, long)
	}
}

type catalogBackend struct{ *fakeBackend }

func (b catalogBackend) PageEntries(ctx context.Context, q backend.PageQuery) (backend.PageEntriesResult, error) {
	switch q.URL {
	case backend.ProductCategoriesURL:
		return backend.PageEntriesResult{Entries: []backend.PageEntry{
			{Kind: backend.EntryCategory, ID: "hot", Title: "热门分类", URL: "/page?url=hot"},
			{Kind: backend.EntryCategory, ID: "tablet", Title: "平板分类", URL: "/page?url=tablet"},
		}}, nil
	case "/page?url=hot":
		return backend.PageEntriesResult{Entries: []backend.PageEntry{
			{Kind: backend.EntryProduct, ID: "1", Title: "手机甲", Hot: "3150"},
			{Kind: backend.EntryProduct, ID: "2", Title: "手机乙", Hot: "1.2万"},
			{Kind: backend.EntryProduct, ID: "3", Title: "手机丙", Hot: "1.3万"},
		}}, nil
	case "/page?url=tablet":
		return backend.PageEntriesResult{Entries: []backend.PageEntry{
			{Kind: backend.EntryProduct, ID: "4", Title: "平板甲", Hot: "4850"},
		}}, nil
	}
	return b.fakeBackend.PageEntries(ctx, q)
}

func TestProductCatalogSwitchesCategory(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a := &App{cx: cx, backend: catalogBackend{newFakeBackend()}, images: newImageStore(t.TempDir(), "")}
		return a.productCatalog(cx, a.newPageList(cx, backend.ProductCategoriesURL, "数码库", nil, 0, ""), listSpec{})
	}, nsgotest.Size(400, 800))
	settle(app)
	rail := app.Find("role:tab 平板分类").Bounds()
	left, middle, right := app.Find("text:手机甲").Bounds(), app.Find("text:手机乙").Bounds(), app.Find("text:手机丙").Bounds()
	if !(rail.Origin.X < left.Origin.X && left.Origin.X < middle.Origin.X &&
		middle.Origin.X < right.Origin.X && left.Origin.Y == right.Origin.Y) {
		t.Fatalf("product grid beside category rail:\n%s", app.Dump())
	}
	app.Press("role:tab 平板分类")
	settle(app)
	if !shows(app, "text:平板甲") || shows(app, "text:手机甲") {
		t.Fatalf("selected category did not replace the grid:\n%s", app.Dump())
	}
}

type phoneRatingsBackend struct{ *fakeBackend }

func (b phoneRatingsBackend) PageEntries(ctx context.Context, q backend.PageQuery) (backend.PageEntriesResult, error) {
	if q.URL != backend.DigitalPhoneURL {
		return b.fakeBackend.PageEntries(ctx, q)
	}
	return backend.PageEntriesResult{Entries: []backend.PageEntry{
		{Kind: backend.EntryCard, ID: "device-env", Card: &backend.Card{Kind: backend.CardDeviceEnvironment, Items: []backend.CardItem{
			{Title: "iPhone 15", URL: "/product/15", Rating: backend.ItemRating{Score: "7.3"}},
			{Title: "iOS27", URL: "/t/iOS27", Rating: backend.ItemRating{Score: "8.2", Count: "2986"}},
		}}},
		{Kind: backend.EntryFeed, ID: b.feed.ID, Feed: &b.feed},
	}}, nil
}

func (b phoneRatingsBackend) Search(ctx context.Context, q backend.SearchQuery) (backend.SearchPage, error) {
	if q.Kind == backend.SearchProducts && q.Keyword == "iPhone 17" {
		return backend.SearchPage{Entries: []backend.PageEntry{
			{Kind: backend.EntryProduct, ID: "4283", Title: q.Keyword, Rating: backend.ItemRating{Score: "9.1", Count: "5828", OwnerCount: "4873"}},
		}}, nil
	}
	return b.fakeBackend.Search(ctx, q)
}

func TestPhoneRatingsUseThePhysicalDevice(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a := &App{cx: cx, backend: phoneRatingsBackend{newFakeBackend()}, images: newImageStore(t.TempDir(), "")}
		return a.entryList(cx, a.newPageList(cx, backend.DigitalPhoneURL, "手机", nil, 0, ""), listSpec{})
	}, nsgotest.Size(400, 800), nsgotest.Environment(core.Environment{
		OS: core.OSIOS, Idiom: core.IdiomPhone, DeviceModel: "iPhone18,3", OSVersion: core.OSVersion{Major: 27},
	}))
	settle(app)
	for _, label := range []string{"我的机型", "我的系统", "iPhone 17", "iOS27", "9.1", "8.2", "4873机主点评", "2986人点评"} {
		if !shows(app, "text:"+label) {
			t.Errorf("missing %q", label)
		}
	}
	if shows(app, "text:iPhone 15") || shows(app, "text:7.3") {
		t.Error("showed the reported model's rating")
	}
	phone := app.Find("role:button 查看我的机型点评").Bounds()
	system := app.Find("role:button 查看我的系统点评").Bounds()
	if phone.Origin.X >= system.Origin.X || phone.Origin.Y != system.Origin.Y {
		t.Fatalf("review actions should share a row: phone %v, system %v", phone, system)
	}
}
