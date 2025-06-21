.PHONY: lint test install upgrade uninstall template dry-run

RELEASE_NAME ?= cosmolet
NAMESPACE ?= network-system
VALUES_FILE ?= values.yaml

# Lint the chart
lint:
	helm lint .

# Test the chart (lint + template)
test: lint template

# Install the chart
install:
	helm install $(RELEASE_NAME) . \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--values $(VALUES_FILE)

# Upgrade the chart
upgrade:
	helm upgrade $(RELEASE_NAME) . \
		--namespace $(NAMESPACE) \
		--values $(VALUES_FILE)

# Uninstall the chart
uninstall:
	helm uninstall $(RELEASE_NAME) --namespace $(NAMESPACE)

# Generate templates (for debugging)
template:
	helm template $(RELEASE_NAME) . \
		--namespace $(NAMESPACE) \
		--values $(VALUES_FILE)

# Dry run installation
dry-run:
	helm install $(RELEASE_NAME) . \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--values $(VALUES_FILE) \
		--dry-run

# Package the chart
package:
	helm package .

# Show values
show-values:
	helm show values .

# Production install
install-prod:
	helm install $(RELEASE_NAME) . \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--values values-production.yaml

# Development install
install-dev:
	helm install $(RELEASE_NAME) . \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--values values-dev.yaml
