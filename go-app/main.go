// orcapod-ingress-provisioner is a CLI tool used by OrcaPod's platform
// team to provision and manage standardized Ingress resources for tenant
// applications. Each tenant gets a consistent set of Ingress resources with
// the company's default security headers, rate limits, and auth configuration.
//
// This tool needs to be migrated from nginx-ingress to Gateway API before
// the March 2026 retirement deadline.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Failed to build kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create clientset: %v", err)
	}

	ctx := context.Background()
	manager := NewIngressManager(clientset)

	// Ensure the nginx IngressClass exists before provisioning
	if err := manager.EnsureIngressClass(ctx); err != nil {
		log.Fatalf("Failed to ensure IngressClass: %v", err)
	}

	// Provision the storefront tenant — public-facing web app with API backend
	if err := provisionStorefront(ctx, manager); err != nil {
		log.Fatalf("Failed to provision storefront: %v", err)
	}

	// Provision the internal admin dashboard — behind OAuth proxy
	if err := provisionAdminDashboard(ctx, manager); err != nil {
		log.Fatalf("Failed to provision admin dashboard: %v", err)
	}

	// Set up canary deployment for the new storefront v2
	if err := provisionStorefrontCanary(ctx, manager); err != nil {
		log.Fatalf("Failed to provision canary: %v", err)
	}

	// List all provisioned ingresses for verification
	ingresses, err := manager.ListIngresses(ctx, "storefront")
	if err != nil {
		log.Fatalf("Failed to list ingresses: %v", err)
	}
	fmt.Println("=== Provisioned Ingresses ===")
	for _, ing := range ingresses {
		fmt.Printf("  %s (class: %s)\n", ing.Name, getIngressClassName(&ing))
	}
}

// provisionStorefront creates the ingress resources for the public storefront.
// The storefront has a web frontend, API backend, and docs site — all behind
// TLS with standard security headers and rate limiting.
func provisionStorefront(ctx context.Context, m *IngressManager) error {
	ingress := m.BuildBasicIngress("storefront", "storefront", "shop.orcapod.io", "/", "web-frontend", 8080)

	// TLS termination at the ingress
	ingress.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
	ingress.Spec.TLS = []networkingv1.IngressTLS{
		{
			Hosts:      []string{"shop.orcapod.io"},
			SecretName: "shop-orcapod-tls",
		},
	}

	// Standard rate limiting for the public endpoint
	ingress.Annotations["nginx.ingress.kubernetes.io/limit-rps"] = "50"

	// Security headers via configuration-snippet
	m.SetCustomHeaders(ingress, map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
	})

	// CORS for the storefront API
	ingress.Annotations["nginx.ingress.kubernetes.io/enable-cors"] = "true"
	ingress.Annotations["nginx.ingress.kubernetes.io/cors-allow-origin"] = "https://shop.orcapod.io"
	ingress.Annotations["nginx.ingress.kubernetes.io/cors-allow-methods"] = "GET, POST, PUT, DELETE, OPTIONS"

	if err := ValidateIngress(ingress); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	created, err := m.CreateIngress(ctx, ingress)
	if err != nil {
		return fmt.Errorf("failed to create storefront ingress: %w", err)
	}
	fmt.Printf("Created storefront ingress: %s\n", created.Name)

	// Separate ingress for the API with higher body size limit (file uploads)
	apiIngress := m.BuildBasicIngress("storefront-api", "storefront", "api.orcapod.io", "/", "api-backend", 8080)
	apiIngress.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
	apiIngress.Annotations["nginx.ingress.kubernetes.io/proxy-body-size"] = "25m"
	apiIngress.Annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"] = "120"
	apiIngress.Annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = "120"
	apiIngress.Spec.TLS = []networkingv1.IngressTLS{
		{
			Hosts:      []string{"api.orcapod.io"},
			SecretName: "api-orcapod-tls",
		},
	}

	created, err = m.CreateIngress(ctx, apiIngress)
	if err != nil {
		return fmt.Errorf("failed to create API ingress: %w", err)
	}
	fmt.Printf("Created API ingress: %s\n", created.Name)

	return nil
}

// provisionAdminDashboard creates an ingress for the internal admin dashboard.
// It's behind OAuth2 proxy (external auth) and restricted to VPN IP ranges.
func provisionAdminDashboard(ctx context.Context, m *IngressManager) error {
	ingress := m.BuildIngressWithExternalAuth(
		"admin-dashboard", "admin",
		"admin.orcapod.io", "/",
		"admin-ui", 3000,
	)

	// Restrict to VPN CIDR
	m.AddWhitelistSourceRange(ingress, "10.0.0.0/8, 172.16.0.0/12")

	// TLS
	ingress.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
	ingress.Annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] = "true"
	ingress.Spec.TLS = []networkingv1.IngressTLS{
		{
			Hosts:      []string{"admin.orcapod.io"},
			SecretName: "admin-orcapod-tls",
		},
	}

	// Session affinity — admin dashboard is stateful
	ingress.Annotations["nginx.ingress.kubernetes.io/affinity"] = "cookie"
	ingress.Annotations["nginx.ingress.kubernetes.io/session-cookie-name"] = "ADMIN_SESSION"
	ingress.Annotations["nginx.ingress.kubernetes.io/session-cookie-max-age"] = "3600"

	// HSTS
	m.SetHSTS(ingress, 31536000, true)

	created, err := m.CreateIngress(ctx, ingress)
	if err != nil {
		return fmt.Errorf("failed to create admin ingress: %w", err)
	}
	fmt.Printf("Created admin ingress: %s\n", created.Name)
	return nil
}

// provisionStorefrontCanary sets up a canary deployment — 10% of traffic goes
// to the new version for gradual rollout.
func provisionStorefrontCanary(ctx context.Context, m *IngressManager) error {
	canary := m.BuildCanaryIngress(
		"storefront-canary", "storefront",
		"shop.orcapod.io", "/",
		"web-frontend-v2", 8080,
		10, // 10% canary weight
	)

	canary.Spec.TLS = []networkingv1.IngressTLS{
		{
			Hosts:      []string{"shop.orcapod.io"},
			SecretName: "shop-orcapod-tls",
		},
	}

	created, err := m.CreateIngress(ctx, canary)
	if err != nil {
		return fmt.Errorf("failed to create canary ingress: %w", err)
	}
	fmt.Printf("Created canary ingress: %s (weight: 10%%)\n", created.Name)
	return nil
}

func getIngressClassName(ingress *networkingv1.Ingress) string {
	if ingress.Spec.IngressClassName != nil {
		return *ingress.Spec.IngressClassName
	}
	if class, ok := ingress.Annotations["kubernetes.io/ingress.class"]; ok {
		return class
	}
	return "<none>"
}
