.PHONY: setup install-target-allocator uninstall-target-allocator install-prometheus uninstall-prometheus install-otel-collector uninstall-otel-collector help

help:
	@echo "Available targets:"
	@echo "  setup                     - Install all components (Target Allocator, Prometheus, OTEL Collector)"
	@echo "  install-target-allocator  - Install OpenTelemetry Target Allocator using Helm"
	@echo "  uninstall-target-allocator - Uninstall OpenTelemetry Target Allocator"
	@echo "  install-prometheus        - Install Prometheus remote write receiver"
	@echo "  uninstall-prometheus      - Uninstall Prometheus remote write receiver"
	@echo "  install-otel-collector    - Install OpenTelemetry Collector using Helm"
	@echo "  uninstall-otel-collector  - Uninstall OpenTelemetry Collector"
	@echo "  help                      - Show this help message"

setup: install-target-allocator install-prometheus install-otel-collector
	@echo "Setup completed! All components have been installed."

install-target-allocator:
	@echo "Installing OpenTelemetry Target Allocator..."
	helm upgrade --install target-allocator open-telemetry/opentelemetry-target-allocator -f values-ta.yaml
	@echo "Target Allocator installation completed!"

uninstall-target-allocator:
	@echo "Uninstalling OpenTelemetry Target Allocator..."
	helm uninstall target-allocator
	@echo "Target Allocator uninstallation completed!"

install-prometheus:
	@echo "Installing Prometheus remote write receiver..."
	kubectl apply -f prometheus.yaml
	@echo "Prometheus installation completed!"

uninstall-prometheus:
	@echo "Uninstalling Prometheus remote write receiver..."
	kubectl delete -f prometheus.yaml
	@echo "Prometheus uninstallation completed!"

install-otel-collector:
	@echo "Installing OpenTelemetry Collector..."
	helm upgrade --install opentelemetry-collector open-telemetry/opentelemetry-collector -f otel-values.yaml
	@echo "OpenTelemetry Collector installation completed!"

uninstall-otel-collector:
	@echo "Uninstalling OpenTelemetry Collector..."
	helm uninstall opentelemetry-collector
	@echo "OpenTelemetry Collector uninstallation completed!" 