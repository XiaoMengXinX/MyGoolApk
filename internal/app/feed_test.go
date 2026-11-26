package app

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestFeedFooterWrapsCountsWhenTopicsDoNotFit(t *testing.T) {
	longTopic := strings.Repeat("很长的话题", 15)
	for _, tc := range []struct {
		name, topic, relation string
		wrap                  bool
	}{
		{name: "short topic", topic: "手机"},
		{name: "short pair", topic: "手机", relation: "评测"},
		{name: "long topic", topic: longTopic, wrap: true},
		{name: "long pair", topic: longTopic, relation: "评测", wrap: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			feed := backend.Feed{Target: &backend.Target{Title: tc.topic},
				Stats: backend.Stats{Likes: 1000, Comments: 80, Forwards: 5}}
			if tc.relation != "" {
				feed.Relation = &backend.CardItem{Title: tc.relation}
			}
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				a := &App{}
				return a.feedFooter(cx, func() backend.Feed { return feed }, func() {})
			})
			topic := app.Find("role:button " + tc.topic).Bounds()
			counts := app.Find("role:button 查看帖子").Bounds()
			if wrapped := counts.Origin.Y > topic.Origin.Y; wrapped != tc.wrap {
				t.Errorf("counts at %v, topic at %v, want wrapped = %v", counts, topic, tc.wrap)
			}
			if !tc.wrap && topic.Origin.X+topic.Size.Width > counts.Origin.X {
				t.Errorf("topic %v overlaps counts %v", topic, counts)
			}
			if tc.relation != "" {
				relation := app.Find("role:button " + tc.relation).Bounds()
				if relation.Origin.Y != topic.Origin.Y || relation.Origin.X+relation.Size.Width > counts.Origin.X+counts.Size.Width {
					t.Errorf("relation %v does not fit in the topic row beside %v", relation, topic)
				}
				if !tc.wrap && relation.Origin.X+relation.Size.Width > counts.Origin.X {
					t.Errorf("relation %v overlaps counts %v", relation, counts)
				}
			}
		})
	}
}
