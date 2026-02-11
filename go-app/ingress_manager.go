package main

import (
	"context"
	"fmt"
	"strconv"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

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

// BuildTLSIngress creates an Ingress with TLS termination.
func (m *IngressManager) BuildTLSIngress(name, namespace, host, path, serviceName string, servicePort int32, tlsSecretName string) *networkingv1.Ingress {
	ingress := m.BuildBasicIngress(name, namespace, host, path, serviceName, servicePort)

	// Add nginx-specific TLS annotations
	ingress.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
	ingress.Annotations["nginx.ingress.kubernetes.io/force-ssl-redirect"] = "true"
	ingress.Annotations["nginx.ingress.kubernetes.io/ssl-passthrough"] = "false"

	// Add TLS spec
	ingress.Spec.TLS = []networkingv1.IngressTLS{
		{
			Hosts:      []string{host},
			SecretName: tlsSecretName,
		},
	}

	return ingress
}

// BuildAnnotatedNginxIngress creates an Ingress with common nginx-ingress annotations
// including URL rewriting, proxy tuning, CORS, and rate limiting.
// The path is wrapped in a regex capture group so that rewrite-target /$2 works
// correctly (e.g. path "/api" becomes "/api(/|$)(.*)").
func (m *IngressManager) BuildAnnotatedNginxIngress(name, namespace, host, path, serviceName string, servicePort int32) *networkingv1.Ingress {
	nginxClass := "nginx"
	pathType := networkingv1.PathTypeImplementationSpecific
	rewritePath := path + "(/|$)(.*)"

	ingress := &networkingv1.Ingress{
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
									Path:     rewritePath,
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

	// Rewrite rules — $2 captures everything after the path prefix
	ingress.Annotations["nginx.ingress.kubernetes.io/rewrite-target"] = "/$2"
	ingress.Annotations["nginx.ingress.kubernetes.io/use-regex"] = "true"

	// Proxy settings
	ingress.Annotations["nginx.ingress.kubernetes.io/proxy-body-size"] = "50m"
	ingress.Annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"] = "3600"
	ingress.Annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = "3600"
	ingress.Annotations["nginx.ingress.kubernetes.io/proxy-connect-timeout"] = "60"

	// CORS
	ingress.Annotations["nginx.ingress.kubernetes.io/enable-cors"] = "true"
	ingress.Annotations["nginx.ingress.kubernetes.io/cors-allow-origin"] = "https://shop.orcapod.io"
	ingress.Annotations["nginx.ingress.kubernetes.io/cors-allow-methods"] = "GET, POST, PUT, DELETE, OPTIONS"
	ingress.Annotations["nginx.ingress.kubernetes.io/cors-allow-headers"] = "Content-Type, Authorization"

	// Rate limiting
	ingress.Annotations["nginx.ingress.kubernetes.io/limit-rps"] = "100"
	ingress.Annotations["nginx.ingress.kubernetes.io/limit-rpm"] = "1000"

	// Custom configuration snippet (security risk)
	ingress.Annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = `
		more_set_headers "X-Custom-Header: my-value";
		proxy_set_header X-Forwarded-Proto $scheme;
	`

	return ingress
}

// BuildCanaryIngress creates an Ingress configured for canary deployments.
func (m *IngressManager) BuildCanaryIngress(name, namespace, host, path, serviceName string, servicePort int32, weightPercent int) *networkingv1.Ingress {
	ingress := m.BuildBasicIngress(name, namespace, host, path, serviceName, servicePort)

	ingress.Annotations["nginx.ingress.kubernetes.io/canary"] = "true"
	ingress.Annotations["nginx.ingress.kubernetes.io/canary-weight"] = strconv.Itoa(weightPercent)
	ingress.Annotations["nginx.ingress.kubernetes.io/canary-by-header"] = "X-Canary"
	ingress.Annotations["nginx.ingress.kubernetes.io/canary-by-header-value"] = "always"

	return ingress
}

// BuildIngressWithDefaultBackend creates an Ingress with a default backend.
func (m *IngressManager) BuildIngressWithDefaultBackend(name, namespace, serviceName string, servicePort int32) *networkingv1.Ingress {
	nginxClass := "nginx"

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
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: serviceName,
					Port: networkingv1.ServiceBackendPort{
						Number: servicePort,
					},
				},
			},
		},
	}
}

// BuildIngressWithAuth creates an Ingress with basic auth annotations.
func (m *IngressManager) BuildIngressWithAuth(name, namespace, host, path, serviceName string, servicePort int32) *networkingv1.Ingress {
	ingress := m.BuildBasicIngress(name, namespace, host, path, serviceName, servicePort)

	// Basic auth annotations
	ingress.Annotations["nginx.ingress.kubernetes.io/auth-type"] = "basic"
	ingress.Annotations["nginx.ingress.kubernetes.io/auth-secret"] = "basic-auth-secret"
	ingress.Annotations["nginx.ingress.kubernetes.io/auth-realm"] = "Authentication Required"

	return ingress
}

// BuildIngressWithExternalAuth creates an Ingress with external auth (OAuth proxy).
func (m *IngressManager) BuildIngressWithExternalAuth(name, namespace, host, path, serviceName string, servicePort int32) *networkingv1.Ingress {
	ingress := m.BuildBasicIngress(name, namespace, host, path, serviceName, servicePort)

	// External auth annotations
	ingress.Annotations["nginx.ingress.kubernetes.io/auth-url"] = "https://sso.orcapod.io/oauth2/auth"
	ingress.Annotations["nginx.ingress.kubernetes.io/auth-signin"] = "https://sso.orcapod.io/oauth2/start?rd=$escaped_request_uri"
	ingress.Annotations["nginx.ingress.kubernetes.io/auth-response-headers"] = "X-Auth-User, X-Auth-Email"

	return ingress
}

// BuildIngressWithSessionAffinity creates an Ingress with session affinity.
func (m *IngressManager) BuildIngressWithSessionAffinity(name, namespace, host, path, serviceName string, servicePort int32) *networkingv1.Ingress {
	ingress := m.BuildBasicIngress(name, namespace, host, path, serviceName, servicePort)

	ingress.Annotations["nginx.ingress.kubernetes.io/affinity"] = "cookie"
	ingress.Annotations["nginx.ingress.kubernetes.io/affinity-mode"] = "persistent"
	ingress.Annotations["nginx.ingress.kubernetes.io/session-cookie-name"] = "SERVERID"
	ingress.Annotations["nginx.ingress.kubernetes.io/session-cookie-max-age"] = "3600"

	return ingress
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

// SetServerSnippet adds a server-snippet annotation to an existing ingress.
// WARNING: server-snippet is a known security risk in nginx-ingress.
func (m *IngressManager) SetServerSnippet(ingress *networkingv1.Ingress, snippet string) {
	if ingress.Annotations == nil {
		ingress.Annotations = make(map[string]string)
	}
	ingress.Annotations["nginx.ingress.kubernetes.io/server-snippet"] = snippet
}

// AddWhitelistSourceRange restricts access to the ingress by source IP range.
func (m *IngressManager) AddWhitelistSourceRange(ingress *networkingv1.Ingress, cidr string) {
	if ingress.Annotations == nil {
		ingress.Annotations = make(map[string]string)
	}
	ingress.Annotations["nginx.ingress.kubernetes.io/whitelist-source-range"] = cidr
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

// SetWebSocketSupport enables WebSocket support for the ingress.
func (m *IngressManager) SetWebSocketSupport(ingress *networkingv1.Ingress) {
	if ingress.Annotations == nil {
		ingress.Annotations = make(map[string]string)
	}
	ingress.Annotations["nginx.ingress.kubernetes.io/proxy-http-version"] = "1.1"
	ingress.Annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"] = "3600"
	ingress.Annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = "3600"
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

// GetIngressStatus returns the load balancer ingress points for an ingress.
func (m *IngressManager) GetIngressStatus(ctx context.Context, namespace, name string) ([]networkingv1.IngressLoadBalancerIngress, error) {
	ingress, err := m.GetIngress(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	return ingress.Status.LoadBalancer.Ingress, nil
}
