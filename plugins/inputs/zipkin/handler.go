package zipkin

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/NYTimes/gziphandler"
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/gorilla/mux"
	"github.com/openzipkin/zipkin-go-opentracing/_thrift/gen-go/zipkincore"
)

// SpanHandler is an implementation of a Handler which accepts zipkin thrift
// span data and sends it to the recorder
type SpanHandler struct {
	Path      string
	recorder  Recorder
	waitGroup *sync.WaitGroup
}

// NewSpanHandler returns a new server instance given path to handle
func NewSpanHandler(path string) *SpanHandler {
	return &SpanHandler{
		Path: path,
	}
}

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set(`Access-Control-Allow-Origin`, origin)
			w.Header().Set(`Access-Control-Allow-Methods`, strings.Join([]string{
				`GET`,
				`OPTIONS`,
			}, ", "))

			w.Header().Set(`Access-Control-Allow-Headers`, strings.Join([]string{
				`Accept`,
				`Accept-Encoding`,
				`Content-Length`,
				`Content-Type`,
			}, ", "))

			w.Header().Set(`Access-Control-Expose-Headers`, strings.Join([]string{
				`Date`,
			}, ", "))
		}

		if r.Method == "OPTIONS" {
			return
		}

		next.ServeHTTP(w, r)
	}
}

// Register implements the Service interface. Register accepts zipkin thrift data
// POSTed to the path of the mux router
func (s *SpanHandler) Register(router *mux.Router, recorder Recorder) error {
	handler := gziphandler.GzipHandler(http.HandlerFunc(s.Spans))
	//TODO: add more cors middleware

	router.Handle(s.Path, handler).Methods("POST")
	s.recorder = recorder
	return nil
}

// Spans handles zipkin thrift spans
func (s *SpanHandler) Spans(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Got request from host: ", r.Host)
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.recorder.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	spans, err := unmarshalThrift(body)
	if err != nil {
		s.recorder.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	trace := NewTrace(spans)

	if err = s.recorder.Record(trace); err != nil {
		s.recorder.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func unmarshalThrift(body []byte) ([]*zipkincore.Span, error) {
	buffer := thrift.NewTMemoryBuffer()
	if _, err := buffer.Write(body); err != nil {
		return nil, err
	}

	transport := thrift.NewTBinaryProtocolTransport(buffer)
	_, size, err := transport.ReadListBegin()
	if err != nil {
		return nil, err
	}

	spans := make([]*zipkincore.Span, size)
	for i := 0; i < size; i++ {
		zs := &zipkincore.Span{}
		if err = zs.Read(transport); err != nil {
			return nil, err
		}
		spans[i] = zs
	}

	if err = transport.ReadListEnd(); err != nil {
		return nil, err
	}

	return spans, nil
}
