package customhttp

import "net/http"

type HTTPCommand interface {
	Do(req *http.Request) (resp *http.Response, err error)
}

type httpCommandFunc func(req *http.Request) (resp *http.Response, err error)

func (h httpCommandFunc) Do(req *http.Request) (resp *http.Response, err error) {
	return h(req)
}

type HTTPCommandBuilder struct {
	client         HTTPCommand
	circuitBreaker middleware
}

func New(options ...func(*HTTPCommandBuilder)) *HTTPCommandBuilder {
	builder := &HTTPCommandBuilder{
		client:         http.DefaultClient,
		circuitBreaker: noOpsMiddleware(),
	}

	for _, option := range options {
		option(builder)
	}
	return builder
}

func (b *HTTPCommandBuilder) Build() HTTPCommand {
	mw := chainMiddleware(b.circuitBreaker)
	return mw(b.client.Do)
}

// WithHTTPClient allows the user to supply their own http.Client
func WithHTTPClient(client HTTPCommand) func(*HTTPCommandBuilder) {
	return func(builder *HTTPCommandBuilder) {
		builder.client = client
	}
}
