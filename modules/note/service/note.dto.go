package service

// A payload is what the service takes, and it is deliberately not the request
// struct. The request has pointers and binding tags because it describes what
// arrived over HTTP; the payload describes what the operation needs. Keeping
// them apart is what lets a job or another module call the same service without
// constructing a fake HTTP request.
//
// The handler converts one to the other with utils.Copy, which matches by
// field name and dereferences pointers.

type CreatePayload struct {
	Title  string
	Body   string
	Pinned bool
}

type UpdatePayload struct {
	Title  string
	Body   string
	Pinned bool
}
