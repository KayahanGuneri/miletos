package openapi

import (
	_ "embed"
	"miletos-go/internal/infra/http/transport"
	"net/http"
)

//go:embed openapi.json
var openAPIDocument []byte

// DocumentBytes returns an independent copy of the OpenAPI document.
func DocumentBytes() []byte {
	return append(
		[]byte(nil),
		openAPIDocument...,
	)
}

type ErrorWriter func(
	http.ResponseWriter,
	*http.Request,
	*transport.APIError,
)

//go:embed swagger.html
var swaggerHTML string

func OpenAPI(
	writeError ErrorWriter,
	writer http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodGet {
		writer.Header().Set(
			"Allow",
			http.MethodGet,
		)

		writeError(
			writer,
			request,
			transport.NewAPIError(
				http.StatusMethodNotAllowed,
				transport.ErrorCodeMethodNotAllowed,
				"The requested HTTP method is not allowed for this resource.",
			),
		)

		return
	}

	writer.Header().Set(
		"Content-Type",
		"application/json",
	)

	writer.Header().Set(
		"Cache-Control",
		"no-store",
	)

	writer.WriteHeader(
		http.StatusOK,
	)

	_, _ =
		writer.Write(
			openAPIDocument,
		)
}

func SwaggerRedirect(
	writeError ErrorWriter,
	writer http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodGet {
		writer.Header().Set(
			"Allow",
			http.MethodGet,
		)

		writeError(
			writer,
			request,
			transport.NewAPIError(
				http.StatusMethodNotAllowed,
				transport.ErrorCodeMethodNotAllowed,
				"The requested HTTP method is not allowed for this resource.",
			),
		)

		return
	}

	http.Redirect(
		writer,
		request,
		"/swagger/",
		http.StatusPermanentRedirect,
	)
}

func SwaggerUI(
	writeError ErrorWriter,
	writer http.ResponseWriter,
	request *http.Request,
) {
	if request.URL.Path !=
		"/swagger/" {

		writeError(
			writer,
			request,
			transport.NewAPIError(
				http.StatusNotFound,
				transport.ErrorCodeNotFound,
				"The requested resource was not found.",
			),
		)

		return
	}

	if request.Method != http.MethodGet {
		writer.Header().Set(
			"Allow",
			http.MethodGet,
		)

		writeError(
			writer,
			request,
			transport.NewAPIError(
				http.StatusMethodNotAllowed,
				transport.ErrorCodeMethodNotAllowed,
				"The requested HTTP method is not allowed for this resource.",
			),
		)

		return
	}

	writer.Header().Set(
		"Content-Type",
		"text/html; charset=utf-8",
	)

	writer.Header().Set(
		"Cache-Control",
		"no-store",
	)

	writer.WriteHeader(
		http.StatusOK,
	)

	_, _ =
		writer.Write(
			[]byte(
				swaggerHTML,
			),
		)
}
