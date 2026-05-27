# otel-full-mesh

Full-mesh OpenTelemetry lab in the `observability` namespace with upstream Istio Helm charts, strict mTLS, and `REGISTRY_ONLY` outbound traffic.

## Components

- `istio/base` and `istio/istiod` pinned to `1.30.0`
- `PeerAuthentication` with `STRICT` mTLS in `observability`
- `meshConfig.outboundTrafficPolicy.mode=REGISTRY_ONLY`
- Prometheus running as a remote write receiver inside the mesh
- OpenTelemetry Collector scraping in-mesh workloads and remote-writing to Prometheus
- Avalanche generating Prometheus metrics behind a headless service

## Usage

```bash
make setup
make status
make verify-mtls
make portforward-prometheus
```

## Notes

- The `observability` namespace is created and labeled for automatic sidecar injection before workloads are installed.
- Traffic between the collector, Avalanche, and Prometheus stays inside the mesh and uses Istio mTLS automatically.
- Registry-only mode blocks unknown external egress from sidecars, so the lab only relies on Kubernetes service registry destinations.
