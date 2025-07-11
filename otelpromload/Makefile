# ==== Configuration =====================================================
TARGETS ?= 1000
SAMPLES_PER_SEC ?= 2000
CARDINALITY ?= 100
DURATION ?= 300
CLUSTER ?= otel-load-test
TEARDOWN ?= false
HELM_EXTRA_VALUES ?=

.PHONY: setup install-cluster install-prometheus install-otel-collector install-avalanche install-all \
        uninstall-cluster uninstall-prometheus uninstall-otel-collector uninstall-avalanche uninstall-all \
        status logs-otel logs-avalanche logs-prometheus test portforward-prometheus help

help:
	@echo "Available targets:"
	@echo "  setup                     - Install cluster and all components"
	@echo "  install-cluster           - Create k3d cluster"
	@echo "  install-prometheus        - Install Prometheus remote write receiver"
	@echo "  install-otel-collector    - Install OpenTelemetry Collector using Helm"
	@echo "  install-avalanche         - Install Avalanche metrics generator"
	@echo "  install-all               - Install all components (requires existing cluster)"
	@echo "  uninstall-cluster         - Delete k3d cluster"
	@echo "  uninstall-prometheus      - Uninstall Prometheus remote write receiver"
	@echo "  uninstall-otel-collector  - Uninstall OpenTelemetry Collector"
	@echo "  uninstall-avalanche       - Uninstall Avalanche metrics generator"
	@echo "  uninstall-all             - Uninstall all components (keeps cluster)"
	@echo "  status                    - Show status of all pods"
	@echo "  logs-otel                 - Show OpenTelemetry Collector logs"
	@echo "  logs-avalanche            - Show Avalanche logs"
	@echo "  logs-prometheus           - Show Prometheus logs"
	@echo "  portforward-prometheus    - Forward Prometheus to localhost:9090"
	@echo "  test                      - Run load test for specified duration"
	@echo "  help                      - Show this help message"
	@echo ""
	@echo "Configuration (can be overridden):"
	@echo "  TARGETS=$(TARGETS)          - Number of time series per avalanche instance"
	@echo "  CARDINALITY=$(CARDINALITY)        - Number of gauge metrics per instance"
	@echo "  DURATION=$(DURATION)           - Load test duration in seconds"
	@echo "  CLUSTER=$(CLUSTER)   - k3d cluster name"

setup: install-cluster install-all
	@echo "Setup completed! All components have been installed."

install-cluster:
	@echo "Creating k3d cluster..."
	k3d cluster create $(CLUSTER) --agents 2 --k3s-arg "--disable=traefik@server:0"
	@echo "Cluster creation completed!"

install-prometheus:
	@echo "Installing Prometheus remote write receiver..."
	kubectl apply -f prometheus.yaml
	@echo "Prometheus installation completed!"

install-otel-collector:
	@echo "Adding OpenTelemetry Helm repository..."
	helm repo add otel https://open-telemetry.github.io/opentelemetry-helm-charts
	helm repo update
	@echo "Installing OpenTelemetry Collector..."
	helm upgrade --install otel otel/opentelemetry-collector \
		-f values.yaml $(if $(HELM_EXTRA_VALUES),-f $(HELM_EXTRA_VALUES))
	@echo "OpenTelemetry Collector installation completed!"

install-avalanche:
	@echo "Installing Avalanche metrics generator..."
	@echo "This will create 1000 avalanche instances, each with $(TARGETS) time series"
	@echo "Total time series: 1000 × $(TARGETS) = $$(( 1000 * $(TARGETS) ))"
	@sed 's/--gauge-metric-count=100/--gauge-metric-count=$(CARDINALITY)/' avalanche.yaml | \
	sed 's/--series-count=1000/--series-count=$(TARGETS)/' | \
	kubectl apply -f -
	@echo "Avalanche installation completed!"

install-all: install-prometheus install-otel-collector install-avalanche
	@echo "All components installed successfully!"

uninstall-cluster:
	@echo "Deleting k3d cluster..."
	k3d cluster delete $(CLUSTER)
	@echo "Cluster deletion completed!"

uninstall-prometheus:
	@echo "Uninstalling Prometheus remote write receiver..."
	kubectl delete -f prometheus.yaml --ignore-not-found=true
	@echo "Prometheus uninstallation completed!"

uninstall-otel-collector:
	@echo "Uninstalling OpenTelemetry Collector..."
	helm uninstall otel --ignore-not-found
	@echo "OpenTelemetry Collector uninstallation completed!"

uninstall-avalanche:
	@echo "Uninstalling Avalanche metrics generator..."
	kubectl delete -f avalanche.yaml --ignore-not-found=true
	@echo "Avalanche uninstallation completed!"

uninstall-all: uninstall-avalanche uninstall-otel-collector uninstall-prometheus
	@echo "All components uninstalled successfully!"

status:
	@echo "Checking pod status..."
	kubectl get pods -o wide

logs-otel:
	@echo "OpenTelemetry Collector logs:"
	kubectl logs -l app.kubernetes.io/name=opentelemetry-collector --tail=20

logs-avalanche:
	@echo "Avalanche logs:"
	kubectl logs -l app=avalanche --tail=20

logs-prometheus:
	@echo "Prometheus logs:"
	kubectl logs -l app=prometheus --tail=20

portforward-prometheus:
	@echo "Port forwarding Prometheus to localhost:9090..."
	@echo "Access Prometheus at: http://localhost:9090"
	@echo "Press Ctrl+C to stop port forwarding"
	kubectl port-forward service/prometheus 9090:9090

test:
	@echo "Running $(DURATION)s load test..."
	sleep $(DURATION)
	@echo "Load test completed!" 