ISTIO_VERSION ?= 1.30.0
CLUSTER ?= otel-full-mesh
NAMESPACE ?= observability
TARGETS ?= 1000
CARDINALITY ?= 100
HELM_EXTRA_VALUES ?=

.PHONY: setup install-cluster install-namespace install-mtls install-istio install-prometheus install-otel-collector install-avalanche \
	install-all destroy uninstall-cluster uninstall-istio uninstall-prometheus uninstall-otel-collector uninstall-avalanche \
	uninstall-all status verify-mtls logs-otel logs-prometheus logs-avalanche portforward-prometheus help

help:
	@echo "Available targets:"
	@echo "  setup                 - Create cluster and install the full mesh lab"
	@echo "  install-cluster       - Create k3d cluster"
	@echo "  install-namespace     - Create namespace and label it for injection"
	@echo "  install-mtls          - Apply strict mTLS policy after Istio is installed"
	@echo "  install-istio         - Install upstream Istio Helm charts"
	@echo "  install-prometheus    - Install Prometheus remote write receiver"
	@echo "  install-otel-collector - Install OpenTelemetry Collector"
	@echo "  install-avalanche     - Install Avalanche metrics generator"
	@echo "  install-all           - Install all workloads into the mesh"
	@echo "  destroy               - Remove the lab and delete the k3d cluster"
	@echo "  uninstall-cluster     - Delete k3d cluster"
	@echo "  uninstall-istio       - Uninstall Istio control plane"
	@echo "  uninstall-prometheus  - Uninstall Prometheus"
	@echo "  uninstall-otel-collector - Uninstall OpenTelemetry Collector"
	@echo "  uninstall-avalanche   - Uninstall Avalanche"
	@echo "  uninstall-all         - Remove all lab components except the cluster"
	@echo "  status                - Show pods, services, and peer authentication"
	@echo "  verify-mtls           - Check injection, strict mTLS policy, and proxy config"
	@echo "  logs-otel             - Show OpenTelemetry Collector logs"
	@echo "  logs-prometheus       - Show Prometheus logs"
	@echo "  logs-avalanche        - Show Avalanche logs"
	@echo "  portforward-prometheus - Forward Prometheus to localhost:9090"
	@echo ""
	@echo "Configuration (can be overridden):"
	@echo "  ISTIO_VERSION=$(ISTIO_VERSION)"
	@echo "  CLUSTER=$(CLUSTER)"
	@echo "  NAMESPACE=$(NAMESPACE)"
	@echo "  TARGETS=$(TARGETS)"
	@echo "  CARDINALITY=$(CARDINALITY)"

setup: install-cluster install-all
	@echo "Setup completed. The full mesh lab is ready."

install-cluster:
	@echo "Creating k3d cluster $(CLUSTER)..."
	@if k3d cluster list $(CLUSTER) >/dev/null 2>&1; then \
		echo "k3d cluster $(CLUSTER) already exists, reusing it."; \
	else \
		k3d cluster create $(CLUSTER) --agents 2 --k3s-arg "--disable=traefik@server:0"; \
	fi
	@echo "Cluster creation completed."

install-namespace:
	@echo "Creating namespace $(NAMESPACE) and labeling it for injection..."
	kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl label namespace $(NAMESPACE) istio-injection=enabled --overwrite
	@echo "Namespace configuration completed."

install-mtls:
	@echo "Applying strict mTLS policy to namespace $(NAMESPACE)..."
	@kubectl get crd peerauthentications.security.istio.io >/dev/null 2>&1 || \
		(printf 'Istio PeerAuthentication CRD is not installed yet. Run make install-istio first.\n' >&2; exit 1)
	kubectl apply -f namespace.yaml
	@echo "Strict mTLS policy applied."

install-istio:
	@echo "Installing upstream Istio Helm charts $(ISTIO_VERSION)..."
	helm repo add istio https://istio-release.storage.googleapis.com/charts
	helm repo update
	helm upgrade --install istio-base istio/base --version $(ISTIO_VERSION) -n istio-system --create-namespace
	helm upgrade --install istiod istio/istiod --version $(ISTIO_VERSION) -n istio-system -f istiod-values.yaml
	kubectl rollout status deployment/istiod -n istio-system --timeout=180s
	@echo "Istio installation completed."

install-prometheus:
	@echo "Installing Prometheus remote write receiver..."
	kubectl apply -f prometheus.yaml
	kubectl rollout status deployment/prometheus -n $(NAMESPACE) --timeout=180s
	@echo "Prometheus installation completed."

install-otel-collector:
	@echo "Installing OpenTelemetry Collector..."
	helm repo add otel https://open-telemetry.github.io/opentelemetry-helm-charts
	helm repo update
	helm upgrade --install otel otel/opentelemetry-collector \
		-n $(NAMESPACE) \
		-f otel-values.yaml $(if $(HELM_EXTRA_VALUES),-f $(HELM_EXTRA_VALUES))
	kubectl rollout status deployment/otel-opentelemetry-collector -n $(NAMESPACE) --timeout=180s
	@echo "OpenTelemetry Collector installation completed."

install-avalanche:
	@echo "Installing Avalanche metrics generator..."
	@echo "Each Avalanche pod will expose $(TARGETS) time series with $(CARDINALITY) gauges."
	@sed 's/--gauge-metric-count=100/--gauge-metric-count=$(CARDINALITY)/' avalanche.yaml | \
		sed 's/--series-count=1000/--series-count=$(TARGETS)/' | \
		kubectl apply -f -
	kubectl rollout status deployment/avalanche -n $(NAMESPACE) --timeout=180s
	@echo "Avalanche installation completed."

install-all: install-namespace install-istio install-mtls install-prometheus install-otel-collector install-avalanche
	@echo "All lab components installed successfully."

destroy: uninstall-all uninstall-cluster
	@echo "Cluster and lab destroyed."

uninstall-avalanche:
	@echo "Uninstalling Avalanche..."
	kubectl delete -f avalanche.yaml --ignore-not-found=true

uninstall-otel-collector:
	@echo "Uninstalling OpenTelemetry Collector..."
	helm uninstall otel -n $(NAMESPACE) --ignore-not-found

uninstall-prometheus:
	@echo "Uninstalling Prometheus..."
	kubectl delete -f prometheus.yaml --ignore-not-found=true

uninstall-istio:
	@echo "Uninstalling Istio..."
	helm uninstall istiod -n istio-system --ignore-not-found
	helm uninstall istio-base -n istio-system --ignore-not-found
	kubectl delete namespace istio-system --ignore-not-found=true

uninstall-all: uninstall-avalanche uninstall-otel-collector uninstall-prometheus uninstall-istio
	@echo "Removing namespace resources..."
	@if kubectl get crd peerauthentications.security.istio.io >/dev/null 2>&1; then \
		kubectl delete -f namespace.yaml --ignore-not-found=true; \
	else \
		echo "PeerAuthentication CRD already removed, skipping mTLS policy cleanup."; \
	fi
	@if kubectl get namespace $(NAMESPACE) >/dev/null 2>&1; then \
		kubectl delete namespace $(NAMESPACE) --ignore-not-found=true; \
	else \
		echo "Namespace $(NAMESPACE) already removed."; \
	fi

uninstall-cluster:
	@echo "Deleting k3d cluster $(CLUSTER)..."
	@if k3d cluster list $(CLUSTER) >/dev/null 2>&1; then \
		k3d cluster delete $(CLUSTER); \
	else \
		echo "k3d cluster $(CLUSTER) does not exist."; \
	fi

status:
	@echo "Namespaces:"
	kubectl get ns istio-system $(NAMESPACE)
	@echo ""
	@echo "Pods:"
	kubectl get pods -n istio-system
	kubectl get pods -n $(NAMESPACE)
	@echo ""
	@echo "Services:"
	kubectl get svc -n $(NAMESPACE)
	@echo ""
	@echo "PeerAuthentication:"
	kubectl get peerauthentication -n $(NAMESPACE)

verify-mtls:
	@echo "Checking namespace injection label..."
	@test "`kubectl get namespace $(NAMESPACE) -o jsonpath='{.metadata.labels.istio-injection}'`" = "enabled"
	@echo "Namespace injection label is enabled."
	@echo "Checking namespace PeerAuthentication mode..."
	@test "`kubectl get peerauthentication default -n $(NAMESPACE) -o jsonpath='{.spec.mtls.mode}'`" = "STRICT"
	@echo "PeerAuthentication is STRICT."
	@echo "Checking workload sidecars..."
	kubectl get pods -n $(NAMESPACE) -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.name}{" "}{end}{"\n"}{end}'
	@MISSING_PODS=`kubectl get pods -n $(NAMESPACE) -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.name}{" "}{end}{"\n"}{end}' | rg -v '(^[^[:space:]]+[[:space:]]+)(.* )?istio-proxy( .*)?$$' || true`; \
	if [ -n "$$MISSING_PODS" ]; then \
		printf 'One or more pods are missing the istio-proxy sidecar:\n%s\n' "$$MISSING_PODS" >&2; \
		exit 1; \
	fi
	@echo "All lab pods include istio-proxy."
	@echo "Checking collector proxy config for Prometheus upstream..."
	@COLLECTOR_POD=`kubectl get pods -n $(NAMESPACE) -l app.kubernetes.io/instance=otel,app.kubernetes.io/name=opentelemetry-collector -o jsonpath='{.items[0].metadata.name}'`; \
	if [ -z "$$COLLECTOR_POD" ]; then \
		printf 'Collector pod not found.\n' >&2; \
		exit 1; \
	fi; \
	kubectl exec -n $(NAMESPACE) $$COLLECTOR_POD -c istio-proxy -- pilot-agent request GET clusters | \
		rg 'outbound\|9090\|\|prometheus\.$(NAMESPACE)\.svc\.cluster\.local' >/dev/null
	@echo "Collector sidecar has Prometheus registered in CDS."

logs-otel:
	kubectl logs deployment/otel-opentelemetry-collector -n $(NAMESPACE) --tail=50

logs-prometheus:
	kubectl logs deployment/prometheus -n $(NAMESPACE) --tail=50

logs-avalanche:
	kubectl logs deployment/avalanche -n $(NAMESPACE) --tail=50

portforward-prometheus:
	@echo "Access Prometheus at http://localhost:9090"
	kubectl port-forward -n $(NAMESPACE) service/prometheus 9090:9090
