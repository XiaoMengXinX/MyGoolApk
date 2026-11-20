package app

import (
	"context"
	"time"

	"github.com/nsgo-dev/nsgo/ui"
)

// background runs work off the graph, then done on it with what work
// returned.
func background[T any](a *App, work func(context.Context) (T, error), done func(T, error)) {
	go func() {
		v, err := work(context.Background())
		a.cx.Post(func() { done(v, err) })
	}()
}

// backgroundErr is background for work that returns only its error.
func backgroundErr(a *App, work func(context.Context) error, done func(error)) {
	background(a, func(ctx context.Context) (struct{}, error) { return struct{}{}, work(ctx) },
		func(_ struct{}, err error) { done(err) })
}

// notice shows a transient message, such as a failed write.
func (a *App) notice(text string) {
	a.toasts.Show(ui.ToastContent{Title: text, Duration: 3 * time.Second})
}
