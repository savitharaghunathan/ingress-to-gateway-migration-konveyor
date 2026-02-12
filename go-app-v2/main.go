// orcapod-ingress-provisioner is a CLI tool used by OrcaPod's platform
// team to provision and manage standardized Ingress resources for tenant
// applications. Each tenant gets a consistent set of Ingress resources with
// the company's default security headers and TLS configuration.
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
	"strconv"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// The storefront has a web frontend and API backend — both behind TLS with
// standard security headers.
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

	// Security headers via configuration-snippet
	m.SetCustomHeaders(ingress, map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
	})

	// HSTS
	m.SetHSTS(ingress, 31536000, true)

	if err := ValidateIngress(ingress); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	created, err := m.CreateIngress(ctx, ingress)
	if err != nil {
		return fmt.Errorf("failed to create storefront ingress: %w", err)
	}
	fmt.Printf("Created storefront ingress: %s\n", created.Name)

	// Separate ingress for the API with custom timeouts
	apiIngress := m.BuildBasicIngress("storefront-api", "storefront", "api.orcapod.io", "/", "api-backend", 8080)
	apiIngress.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
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

func getIngressClassName(ingress *networkingv1.Ingress) string {
	if ingress.Spec.IngressClassName != nil {
		return *ingress.Spec.IngressClassName
	}
	if class, ok := ingress.Annotations["kubernetes.io/ingress.class"]; ok {
		return class
	}
	return "<none>"
}

// IngressManager handles CRUD operations for Kubernetes Ingress resources.
type IngressManager struct {
	clientset kubernetes.Interface
}

// NewIngressManager creates a new IngressManager.
func NewIngressManager(clientset kubernetes.Interface) *IngressManager {
	return &IngressManager{clientset: clientset}
}

// CreateIngress creates a new Ingress resource in the cluster.
func (m *IngressManager) CreateIngress(ctx context.Context, ingress *networkingv1.Ingress) (*networkingv1.Ingress, error) {
	return m.clientset.NetworkingV1().Ingresses(ingress.Namespace).Create(ctx, ingress, metav1.CreateOptions{})
}

// UpdateIngress updates an existing Ingress resource.
func (m *IngressManager) UpdateIngress(ctx context.Context, ingress *networkingv1.Ingress) (*networkingv1.Ingress, error) {
	return m.clientset.NetworkingV1().Ingresses(ingress.Namespace).Update(ctx, ingress, metav1.UpdateOptions{})
}

// DeleteIngress deletes an Ingress resource by name and namespace.
func (m *IngressManager) DeleteIngress(ctx context.Context, namespace, name string) error {
	return m.clientset.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// GetIngress retrieves a specific Ingress by name.
func (m *IngressManager) GetIngress(ctx context.Context, namespace, name string) (*networkingv1.Ingress, error) {
	return m.clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
}

// ListIngresses lists all Ingress resources in a namespace.
func (m *IngressManager) ListIngresses(ctx context.Context, namespace string) ([]networkingv1.Ingress, error) {
	list, err := m.clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// BuildBasicIngress creates an Ingress object with a single host and path rule.
func (m *IngressManager) BuildBasicIngress(name, namespace, host, path, serviceName string, servicePort int32) *networkingv1.Ingress {
	nginxClass := "nginx"
	pathType := networkingv1.PathTypePrefix

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "nginx",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &nginxClass,
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: serviceName,
											Port: networkingv1.ServiceBackendPort{
												Number: servicePort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// CreateIngressClass creates a new IngressClass for nginx.
func (m *IngressManager) CreateIngressClass(ctx context.Context, name string) (*networkingv1.IngressClass, error) {
	ingressClass := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"ingressclass.kubernetes.io/is-default-class": "true",
			},
		},
		Spec: networkingv1.IngressClassSpec{
			Controller: "k8s.io/ingress-nginx",
		},
	}
	return m.clientset.NetworkingV1().IngressClasses().Create(ctx, ingressClass, metav1.CreateOptions{})
}

// EnsureIngressClass checks if the nginx IngressClass exists and creates it if not.
func (m *IngressManager) EnsureIngressClass(ctx context.Context) error {
	_, err := m.clientset.NetworkingV1().IngressClasses().Get(ctx, "nginx", metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to check IngressClass: %w", err)
		}
		_, err = m.CreateIngressClass(ctx, "nginx")
		if err != nil {
			return fmt.Errorf("failed to create IngressClass: %w", err)
		}
	}
	return nil
}

// SetCustomHeaders adds a configuration-snippet for custom response headers.
func (m *IngressManager) SetCustomHeaders(ingress *networkingv1.Ingress, headers map[string]string) {
	snippet := ""
	for k, v := range headers {
		snippet += fmt.Sprintf("more_set_headers \"%s: %s\";\n", k, v)
	}
	if ingress.Annotations == nil {
		ingress.Annotations = make(map[string]string)
	}
	ingress.Annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = snippet
}

// SetHSTS configures HTTP Strict Transport Security via annotations.
func (m *IngressManager) SetHSTS(ingress *networkingv1.Ingress, maxAge int, includeSubdomains bool) {
	if ingress.Annotations == nil {
		ingress.Annotations = make(map[string]string)
	}
	ingress.Annotations["nginx.ingress.kubernetes.io/hsts"] = "true"
	ingress.Annotations["nginx.ingress.kubernetes.io/hsts-max-age"] = strconv.Itoa(maxAge)
	if includeSubdomains {
		ingress.Annotations["nginx.ingress.kubernetes.io/hsts-include-subdomains"] = "true"
	}
}

// ValidateIngress performs basic validation on an Ingress resource.
func ValidateIngress(ingress *networkingv1.Ingress) error {
	if ingress.Name == "" {
		return fmt.Errorf("ingress name cannot be empty")
	}

	if ingress.Spec.IngressClassName == nil {
		if _, ok := ingress.Annotations["kubernetes.io/ingress.class"]; !ok {
			return fmt.Errorf("ingress must specify an IngressClass via spec.ingressClassName or annotation")
		}
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				return fmt.Errorf("ingress path %s must specify a backend service", path.Path)
			}
		}
	}

	return nil
}

// IngressToString provides a human-readable summary of an Ingress resource.
func IngressToString(ingress *networkingv1.Ingress) string {
	summary := fmt.Sprintf("Ingress: %s/%s\n", ingress.Namespace, ingress.Name)
	if ingress.Spec.IngressClassName != nil {
		summary += fmt.Sprintf("  Class: %s\n", *ingress.Spec.IngressClassName)
	}
	for _, rule := range ingress.Spec.Rules {
		summary += fmt.Sprintf("  Host: %s\n", rule.Host)
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				summary += fmt.Sprintf("    Path: %s -> %s:%d\n",
					path.Path,
					path.Backend.Service.Name,
					path.Backend.Service.Port.Number,
				)
			}
		}
	}
	if len(ingress.Spec.TLS) > 0 {
		for _, tls := range ingress.Spec.TLS {
			summary += fmt.Sprintf("  TLS: %v (secret: %s)\n", tls.Hosts, tls.SecretName)
		}
	}
	return summary
}
