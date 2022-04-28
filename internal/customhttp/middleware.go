package customhttp

import "net/http"

type middleware func(next httpCommandFunc) httpCommandFunc

func chainMiddleware(m ...middleware) middleware {
	return func(final httpCommandFunc) httpCommandFunc {
		last := final
		for i := len(m) - 1; i >= 0; i-- {
			last = m[i](last)
		}

		return func(req *http.Request) (resp *http.Response, err error) {
			return last(req)
		}
	}
}

func noOpsMiddleware() middleware {
	return func(next httpCommandFunc) httpCommandFunc {
		return func(req *http.Request) (resp *http.Response, err error) {
			return next(req)
		}
	}
}
