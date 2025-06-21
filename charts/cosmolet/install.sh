#!/bin/bash

# Cosmolet Installation Script

set -e

RELEASE_NAME="${1:-cosmolet}"
NAMESPACE="${2:-network-system}"
VALUES_FILE="${3:-values.yaml}"

echo "🚀 Installing Cosmolet..."
echo "Release Name: $RELEASE_NAME"
echo "Namespace: $NAMESPACE" 
echo "Values File: $VALUES_FILE"

# Check if Helm is installed
if ! command -v helm &> /dev/null; then
    echo "❌ Helm is not installed. Please install Helm first."
    exit 1
fi

# Check if kubectl is installed
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed. Please install kubectl first."
    exit 1
fi

# Validate Helm chart
echo "🔍 Validating Helm chart..."
helm lint .

# Dry run first
echo "🧪 Performing dry run..."
helm install "$RELEASE_NAME" . \
    --namespace "$NAMESPACE" \
    --create-namespace \
    --values "$VALUES_FILE" \
    --dry-run

# Confirm installation
read -p "❓ Proceed with installation? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "⚡ Installing..."
    helm install "$RELEASE_NAME" . \
        --namespace "$NAMESPACE" \
        --create-namespace \
        --values "$VALUES_FILE"
    
    echo "✅ Installation complete!"
    echo "📊 Check status:"
    echo "   kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=cosmolet"
    echo "   kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=cosmolet"
else
    echo "❌ Installation cancelled."
fi
