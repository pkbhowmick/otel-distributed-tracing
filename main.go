package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	traceIdKey = "x-trace-id"
)

type idGenerator struct {
}

// NewSpanID returns a non-zero span ID from a randomly-chosen sequence.
func (_ *idGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	sid, err := trace.SpanIDFromHex("4bf92f3577b34da6")
	if err != nil {
		panic(err)
	}

	return sid
}

// NewIDs returns a non-zero trace ID and a non-zero span ID from a
// randomly-chosen sequence.
func (_ *idGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	tid := trace.TraceID{}
	traceIdHeader := ctx.Value(traceIdKey)
	if traceIdHeader != "" {
		ctid, err := trace.TraceIDFromHex(traceIdHeader.(string))
		if err == nil {
			tid = ctid
		}
	}

	sid, err := trace.SpanIDFromHex("4bf92f3577b34da6")
	if err != nil {
		panic(err)
	}

	return tid, sid
}

func addTracing(tracerName, spanName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			pctx := context.WithValue(r.Context(), traceIdKey, r.Header.Get(traceIdKey))
			ctx, span := otel.Tracer(tracerName).Start(pctx, spanName)
			defer span.End()

			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}

func initTracing(filename string) (*sdktrace.TracerProvider, error) {
	f, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create the trace file, error: %s", err.Error())
	}

	exp, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
		stdouttrace.WithWriter(f),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create the collector exporter, error: %s", err.Error())
	}

	res, err := resource.New(
		context.Background(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create the otel resource, error: %s", err.Error())
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithIDGenerator(&idGenerator{}),
	), nil
}

func main() {
	tp, err := initTracing("trace.out")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Println(err.Error())
		}
	}()
	otel.SetTracerProvider(tp)

	r := chi.NewRouter()
	r.With(addTracing("healthHandler", "health")).Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	if err := http.ListenAndServe(":8080", r); err != nil {
		panic(err)
	}
}
