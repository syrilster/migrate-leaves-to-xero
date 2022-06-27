// Package context provides functionality to clone an existing context
package context

import (
	"context"
	"time"
)

type DetachedContext struct {
	parent context.Context
}

// Detach is to create a clone of the existing context. This new context does not cancel when the parent context cancels.
func Detach(ctx context.Context) context.Context {
	return DetachedContext{ctx}
}

// Deadline returns the time when work done on behalf of this context should be completed.
func (d DetachedContext) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

// Done returns a channel that's closed when work done on behalf of this context should be cancelled.
func (d DetachedContext) Done() <-chan struct{} {
	return nil
}

func (d DetachedContext) Err() error {
	return nil
}

func (d DetachedContext) Value(key any) any {
	return d.parent.Value(key)
}
