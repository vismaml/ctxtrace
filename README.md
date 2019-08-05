# Istio Trace Propagation via GRPC Inceptors

[![Go Report Card](https://goreportcard.com/badge/github.com/e-conomic/ctxtrace)](https://goreportcard.com/report/github.com/e-conomic/ctxtrace)
[![go-doc](https://godoc.org/github.com/e-conomic/ctxtrace?status.svg)](https://godoc.org/github.com/e-conomic/ctxtrace)

This project will inject the istio tracing headers (which are actually B3 headers) into the context and propgate it via interceptors.

